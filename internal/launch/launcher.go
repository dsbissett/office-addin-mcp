package launch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	defaultCDPPort        = 9222
	defaultLaunchTimeout  = 60 * time.Second
	cdpProbeTimeout       = 1 * time.Second
	stopTimeout           = 10 * time.Second
	launcherToolName      = "office-addin-debugging"
	envWebView2ExtraArgs  = "WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS"
	envRemoteDebuggingArg = "--remote-debugging-port"
)

// LaunchOptions controls a sideload run. All fields are optional; zero values
// produce the default behavior (port 9222, ~60s CDP timeout, dev server
// auto-start).
type LaunchOptions struct {
	Port             int
	Timeout          time.Duration
	DevServerTimeout time.Duration
	SkipDevServer    bool
	// Progress, when non-nil, is called with a short human-readable status
	// line at each launch phase boundary (dev server, sideload, CDP wait).
	// Lets callers stream progress during the long internal waits. Nil-safe.
	Progress func(message string)
}

// LaunchResult is what the caller gets back after a successful sideload.
type LaunchResult struct {
	PID           int      `json:"pid"`
	CDPURL        string   `json:"cdpUrl"`
	ManifestPath  string   `json:"manifestPath"`
	DevServerPort int      `json:"devServerPort,omitempty"`
	Output        []string `json:"output,omitempty"`
	// Source records how the endpoint was obtained: "launched" (a fresh
	// office-addin-debugging spawn), "reused" (a still-alive tracked launch),
	// or "preexisting" (an unrelated Excel already on the port, via
	// LaunchIfNeeded). Lets callers tell a real spawn from a no-op.
	Source string `json:"source,omitempty"`
	// CDPVerified is true when a live /json/version probe confirmed the CDP
	// endpoint actually responded — i.e. this is not a phantom success built
	// from a stale tracked-launch record.
	CDPVerified bool `json:"cdpVerified"`
}

// LaunchError carries a coarse machine-readable reason plus captured child
// output so MCP callers can surface useful diagnostics.
type LaunchError struct {
	Reason  string
	Message string
	Output  []string
}

func (e *LaunchError) Error() string {
	if len(e.Output) == 0 {
		return e.Message
	}
	return e.Message + "\n" + strings.Join(e.Output, "\n")
}

// Reason values surfaced through LaunchError.Reason. Stable strings; tools
// expose them in their error envelope's details.
const (
	ReasonUnsupportedPlatform = "unsupported-platform"
	ReasonLauncherMissing     = "launcher-missing"
	ReasonPortAlreadyConfig   = "port-already-configured"
	ReasonLaunchFailed        = "launch-failed"
	ReasonCDPNotReady         = "cdp-not-ready"
	ReasonDevServerNotReady   = "dev-server-not-ready"
	ReasonStopFailed          = "stop-failed"
	ReasonAborted             = "aborted"
)

// LaunchExcel sideloads project's manifest into Excel via
// office-addin-debugging with WebView2 remote debugging enabled. Returns the
// existing tracked launch if one is already running for the same manifest.
//
// On error the (caller-visible) Reason describes the failure phase so MCP
// tools can categorize the envelope error sensibly.
func LaunchExcel(ctx context.Context, project *Project, opts LaunchOptions) (*LaunchResult, error) {
	if runtime.GOOS != "windows" {
		return nil, &LaunchError{Reason: ReasonUnsupportedPlatform, Message: "launch: WebView2 sideloading is Windows-only"}
	}
	if reused := reuseTrackedLaunch(ctx, project, opts); reused != nil {
		return reused, nil
	}
	return coldStartLaunch(ctx, project, opts)
}

// coldStartLaunch performs a fresh sideload: resolve launcher+env, start the
// dev server, spawn office-addin-debugging, wait for CDP, then track the
// launch and build its result.
func coldStartLaunch(ctx context.Context, project *Project, opts LaunchOptions) (*LaunchResult, error) {
	port := resolveLaunchPort(opts)
	cdpURL := fmt.Sprintf("http://localhost:%d", port)

	launcherCmd, env, err := resolveLauncherEnv(project.Root, port)
	if err != nil {
		return nil, err
	}

	devServer, err := startDevServerIfNeeded(ctx, project, env, opts)
	if err != nil {
		return nil, err
	}

	reportProgress(opts, "Sideloading Excel")
	spawn, err := spawnLauncher(launcherCmd, project, env, devServer)
	if err != nil {
		return nil, err
	}

	reportProgress(opts, "Waiting for CDP endpoint")
	if err := waitForCDPReady(ctx, cdpURL, resolveLaunchTimeout(opts), spawn.exited, spawn.output); err != nil {
		killProcess(spawn.cmd)
		devServer.stop()
		return nil, err
	}

	registerTrackedLaunch(project, cdpURL, launcherCmd, env, spawn, devServer)
	return buildLaunchResult(project, cdpURL, spawn, devServer), nil
}

// resolveLauncherEnv locates the office-addin-debugging launcher and builds the
// child environment, wrapping a missing launcher as ReasonLauncherMissing.
func resolveLauncherEnv(root string, port int) (launcherCmd string, env []string, err error) {
	launcherCmd, err = resolveLauncher(root)
	if err != nil {
		return "", nil, &LaunchError{Reason: ReasonLauncherMissing, Message: err.Error()}
	}
	env, err = buildLaunchEnv(root, port)
	if err != nil {
		return "", nil, err
	}
	return launcherCmd, env, nil
}

// reuseTrackedLaunch returns a "reused" LaunchResult when the manifest already
// has a tracked launch whose process is alive and CDP endpoint responds.
// Otherwise it clears any stale record (full stop) and returns nil so the
// caller performs a cold start.
func reuseTrackedLaunch(ctx context.Context, project *Project, opts LaunchOptions) *LaunchResult {
	existing, ok := defaultRegistry.lookup(project.ManifestPath)
	if !ok {
		return nil
	}
	// Only reuse the tracked launch if the process is alive AND its CDP
	// endpoint responds. After a manual Excel shutdown the registry still
	// holds the old record; returning it blind reports phantom success and
	// never relaunches (the bug behind "called launch but couldn't
	// connect"). The PID check fails fast; the /json/version probe also
	// catches an alive-but-wedged WebView2.
	if processAlive(existing.PID) && ProbeCDPEndpoint(ctx, existing.CDPURL, cdpProbeTimeout).OK {
		reportProgress(opts, "Reusing already-launched Excel")
		return &LaunchResult{
			PID:          existing.PID,
			CDPURL:       existing.CDPURL,
			ManifestPath: project.ManifestPath,
			Source:       "reused",
			CDPVerified:  true,
		}
	}
	// Stale record: Excel is gone. A bare registry delete is NOT enough —
	// the previous office-addin-debugging sideload is still registered, so
	// a fresh `start` would no-op and never reopen Excel. Run the full stop
	// (office-addin-debugging stop + kill launcher + dev server + clear
	// registry) so the relaunch below is a genuine cold start.
	reportProgress(opts, "Tracked Excel is no longer responding; clearing stale launch")
	_ = existing.Stop()
	return nil
}

func resolveLaunchPort(opts LaunchOptions) int {
	if opts.Port <= 0 {
		return defaultCDPPort
	}
	return opts.Port
}

func resolveLaunchTimeout(opts LaunchOptions) time.Duration {
	if opts.Timeout <= 0 {
		return defaultLaunchTimeout
	}
	return opts.Timeout
}

// startDevServerIfNeeded spawns the dev server unless SkipDevServer is set,
// wrapping any failure in a ReasonDevServerNotReady LaunchError.
func startDevServerIfNeeded(ctx context.Context, project *Project, env []string, opts LaunchOptions) (*devServerHandle, error) {
	if opts.SkipDevServer {
		return nil, nil
	}
	reportProgress(opts, "Starting dev server")
	devServer, err := ensureDevServer(ctx, project, env, opts.DevServerTimeout)
	if err != nil {
		return nil, &LaunchError{Reason: ReasonDevServerNotReady, Message: err.Error()}
	}
	return devServer, nil
}

// launchSpawn bundles the started launcher child and its output plumbing.
type launchSpawn struct {
	cmd    *exec.Cmd
	pid    int
	exited <-chan error
	output *outputBuffer
}

// spawnLauncher builds and starts the office-addin-debugging `start` child,
// stopping the dev server and wrapping failures as ReasonLaunchFailed.
func spawnLauncher(launcherCmd string, project *Project, env []string, devServer *devServerHandle) (*launchSpawn, error) {
	cmd, err := buildLauncherCommand(launcherCmd, "start", project, env)
	if err != nil {
		devServer.stop()
		return nil, &LaunchError{Reason: ReasonLaunchFailed, Message: err.Error()}
	}
	output := newOutputBuffer(maxOutputLines)
	attachOutput(cmd, output)
	if err := cmd.Start(); err != nil {
		devServer.stop()
		return nil, &LaunchError{Reason: ReasonLaunchFailed, Message: fmt.Sprintf("spawn %s: %v", launcherToolName, err)}
	}
	return &launchSpawn{cmd: cmd, pid: cmd.Process.Pid, exited: waitChild(cmd), output: output}, nil
}

// registerTrackedLaunch records the live launch in the registry with a StopFn
// that tears down the launcher, dev server, and registry entry.
func registerTrackedLaunch(project *Project, cdpURL, launcherCmd string, env []string, spawn *launchSpawn, devServer *devServerHandle) {
	tracked := &TrackedLaunch{
		Project:   project,
		CDPURL:    cdpURL,
		PID:       spawn.pid,
		Launcher:  launcherCmd,
		devServer: devServer,
	}
	tracked.StopFn = func() error {
		err := stopWithLauncher(launcherCmd, project, env)
		killProcess(spawn.cmd)
		devServer.stop()
		defaultRegistry.delete(project.ManifestPath)
		return err
	}
	defaultRegistry.put(project.ManifestPath, tracked)
}

// buildLaunchResult assembles the success envelope after CDP is confirmed.
func buildLaunchResult(project *Project, cdpURL string, spawn *launchSpawn, devServer *devServerHandle) *LaunchResult {
	res := &LaunchResult{
		PID:          spawn.pid,
		CDPURL:       cdpURL,
		ManifestPath: project.ManifestPath,
		Output:       spawn.output.snapshot(),
		Source:       "launched",
		CDPVerified:  true, // waitForCDPReady above confirmed /json responded
	}
	if devServer != nil {
		res.DevServerPort = devServer.port
	}
	return res
}

// LaunchIfNeeded returns a LaunchResult for the configured port without
// spawning Excel if a CDP-enabled instance is already responding on that
// port. If the probe fails, falls through to LaunchExcel.
//
// The returned `Source` is "preexisting" when no spawn occurred, "launched"
// when this call started Excel. Useful for an MCP entry tool like
// addin.ensureRunning where the agent doesn't care which path was taken,
// only that the endpoint is now reachable.
func LaunchIfNeeded(ctx context.Context, project *Project, opts LaunchOptions) (*LaunchResult, string, error) {
	port := opts.Port
	if port <= 0 {
		port = defaultCDPPort
	}
	cdpURL := fmt.Sprintf("http://localhost:%d", port)
	reportProgress(opts, fmt.Sprintf("Probing CDP endpoint at %s", cdpURL))

	if probe := ProbeCDPEndpoint(ctx, cdpURL, cdpProbeTimeout); probe.OK {
		reportProgress(opts, "Excel already reachable")
		return preexistingResult(project, cdpURL), "preexisting", nil
	}
	if project == nil {
		return nil, "", &LaunchError{
			Reason:  ReasonLaunchFailed,
			Message: fmt.Sprintf("no CDP endpoint at %s and no add-in project supplied to launch", cdpURL),
		}
	}
	res, err := LaunchExcel(ctx, project, opts)
	if err != nil {
		return nil, "", err
	}
	return res, "launched", nil
}

// reportProgress invokes opts.Progress with msg when a callback is set.
func reportProgress(opts LaunchOptions, msg string) {
	if opts.Progress != nil {
		opts.Progress(msg)
	}
}

// preexistingResult builds the LaunchResult for an endpoint that was already
// reachable, tolerating a nil project (manifest path stays empty).
func preexistingResult(project *Project, cdpURL string) *LaunchResult {
	manifestPath := ""
	if project != nil {
		manifestPath = project.ManifestPath
	}
	return &LaunchResult{
		CDPURL:       cdpURL,
		ManifestPath: manifestPath,
		Source:       "preexisting",
		CDPVerified:  true,
	}
}

// StopExcel terminates the active launch for the given manifest. Returns nil
// if there is no active launch (idempotent).
func StopExcel(manifestPath string) error {
	tracked, ok := defaultRegistry.lookup(manifestPath)
	if !ok {
		return nil
	}
	return tracked.Stop()
}

// resolveLauncher picks the best available office-addin-debugging entry
// point: prefer a local node_modules/.bin shim, else fall back to npx on
// PATH with --no-install office-addin-debugging.
func resolveLauncher(root string) (string, error) {
	binDir := localBinDir(root)
	candidates := []string{
		filepath.Join(binDir, launcherToolName+".cmd"),
		filepath.Join(binDir, launcherToolName+".exe"),
		filepath.Join(binDir, launcherToolName),
	}
	for _, c := range candidates {
		if pathExists(c) {
			return c, nil
		}
	}
	if npx, err := exec.LookPath("npx"); err == nil {
		return npx, nil
	}
	return "", fmt.Errorf("%w: install %s as a devDependency in %s or make npx available on PATH",
		errLauncherMissing, launcherToolName, root)
}

// buildLauncherCommand wraps the launcher invocation in a *exec.Cmd. When the
// launcher resolves to npx, we forward --no-install office-addin-debugging
// before the subcommand.
func buildLauncherCommand(launcher string, action string, project *Project, env []string) (*exec.Cmd, error) {
	args := []string{}
	if filepath.Base(launcher) == "npx" || filepath.Base(launcher) == "npx.cmd" || filepath.Base(launcher) == "npx.exe" {
		args = append(args, "--no-install", launcherToolName)
	}
	args = append(args, action, project.ManifestPath)
	cmd := exec.Command(launcher, args...) //nolint:gosec // launcher derived from a fixed allow-list (local shim or npx).
	cmd.Dir = project.Root
	cmd.Env = env
	configurePlatformProcAttr(cmd)
	return cmd, nil
}

// buildLaunchEnv prepares the child's environment: the project's
// node_modules/.bin is prepended to PATH and WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS
// is set to enable CDP. If the user already configured a remote-debugging-port
// elsewhere we refuse rather than fight them for the port.
func buildLaunchEnv(root string, port int) ([]string, error) {
	pairs := os.Environ()
	current := os.Getenv(envWebView2ExtraArgs)
	if remoteDebugRE.MatchString(current) {
		return nil, &LaunchError{
			Reason:  ReasonPortAlreadyConfig,
			Message: fmt.Sprintf("%s already contains %s; unset it before launching Excel from office-addin-mcp", envWebView2ExtraArgs, envRemoteDebuggingArg),
		}
	}
	browserArgs := fmt.Sprintf("%s=%d", envRemoteDebuggingArg, port)
	binDir := localBinDir(root)

	out, pathSet, argsSet := rewriteLaunchEnv(pairs, binDir, browserArgs)
	if !pathSet {
		out = append(out, "PATH="+binDir)
	}
	if !argsSet {
		out = append(out, envWebView2ExtraArgs+"="+browserArgs)
	}
	return out, nil
}

// rewriteLaunchEnv copies pairs while prepending binDir to PATH and replacing
// the WebView2 args entry. It reports whether each of those keys was present so
// the caller can append a default when missing.
func rewriteLaunchEnv(pairs []string, binDir, browserArgs string) (out []string, pathSet, argsSet bool) {
	out = make([]string, 0, len(pairs)+2)
	for _, kv := range pairs {
		key, _, ok := splitEnv(kv)
		switch {
		case !ok:
			out = append(out, kv)
		case strings.EqualFold(key, "PATH"):
			out = append(out, "PATH="+binDir+string(os.PathListSeparator)+envValue(kv))
			pathSet = true
		case strings.EqualFold(key, envWebView2ExtraArgs):
			out = append(out, envWebView2ExtraArgs+"="+browserArgs)
			argsSet = true
		default:
			out = append(out, kv)
		}
	}
	return out, pathSet, argsSet
}

var remoteDebugRE = regexp.MustCompile(`(?i)(^|\s)--remote-debugging-port(\s|=|$)`)

func splitEnv(kv string) (string, string, bool) {
	idx := strings.IndexRune(kv, '=')
	if idx < 0 {
		return "", "", false
	}
	return kv[:idx], kv[idx+1:], true
}

func envValue(kv string) string {
	_, v, _ := splitEnv(kv)
	return v
}

// waitForCDPReady polls /json/version until the endpoint responds with a
// browser version, the deadline elapses, or the launcher child exits early.
func waitForCDPReady(ctx context.Context, cdpURL string, timeout time.Duration, exited <-chan error, output *outputBuffer) error {
	deadline := time.Now().Add(timeout)
	var lastReason string
	for time.Now().Before(deadline) {
		if err := cdpWaitPreempt(ctx, exited, output); err != nil {
			return err
		}
		probe := ProbeCDPEndpoint(ctx, cdpURL, cdpProbeTimeout)
		if probe.OK {
			return nil
		}
		lastReason = probe.Reason
		if err := cdpWaitInterval(ctx, output); err != nil {
			return err
		}
	}
	return &LaunchError{
		Reason:  ReasonCDPNotReady,
		Message: fmt.Sprintf("timed out waiting for %s/json/version (%s)", cdpURL, lastReason),
		Output:  output.snapshot(),
	}
}

// cdpWaitPreempt reports a terminal LaunchError if the context was cancelled
// or the launcher child exited before CDP became ready; otherwise nil.
func cdpWaitPreempt(ctx context.Context, exited <-chan error, output *outputBuffer) error {
	select {
	case <-ctx.Done():
		return &LaunchError{Reason: ReasonAborted, Message: "launch aborted: " + ctx.Err().Error(), Output: output.snapshot()}
	case err := <-exited:
		return &LaunchError{
			Reason:  ReasonLaunchFailed,
			Message: fmt.Sprintf("%s exited before CDP became ready: %v", launcherToolName, err),
			Output:  output.snapshot(),
		}
	default:
		return nil
	}
}

// cdpWaitInterval sleeps one probe interval, returning a terminal LaunchError
// if the context is cancelled while waiting.
func cdpWaitInterval(ctx context.Context, output *outputBuffer) error {
	select {
	case <-time.After(probeInterval):
		return nil
	case <-ctx.Done():
		return &LaunchError{Reason: ReasonAborted, Message: "launch aborted: " + ctx.Err().Error(), Output: output.snapshot()}
	}
}

// stopWithLauncher runs `office-addin-debugging stop <manifest>` with a
// bounded timeout. Any failure is reported as a LaunchError; the caller
// still terminates the child process tree as a backup.
func stopWithLauncher(launcher string, project *Project, env []string) error {
	cmd, err := buildLauncherCommand(launcher, "stop", project, env)
	if err != nil {
		return &LaunchError{Reason: ReasonStopFailed, Message: err.Error()}
	}
	output := newOutputBuffer(maxOutputLines)
	attachOutput(cmd, output)
	if err := cmd.Start(); err != nil {
		return &LaunchError{Reason: ReasonStopFailed, Message: fmt.Sprintf("spawn %s stop: %v", launcherToolName, err)}
	}
	exited := waitChild(cmd)
	select {
	case err := <-exited:
		return stopExitResult(err, output)
	case <-time.After(stopTimeout):
		killProcess(cmd)
		return &LaunchError{Reason: ReasonStopFailed, Message: fmt.Sprintf("timed out waiting for %s stop", launcherToolName), Output: output.snapshot()}
	}
}

// stopExitResult maps the launcher-stop child's exit error to a LaunchError,
// or nil on a clean exit.
func stopExitResult(err error, output *outputBuffer) error {
	if err != nil {
		return &LaunchError{Reason: ReasonStopFailed, Message: fmt.Sprintf("%s stop: %v", launcherToolName, err), Output: output.snapshot()}
	}
	return nil
}

// AsLaunchError extracts a *LaunchError from err if present, returning nil
// otherwise. Helpful for tools that want to surface Reason in envelope
// details without copy-pasting an errors.As at every call site.
func AsLaunchError(err error) *LaunchError {
	var le *LaunchError
	if errors.As(err, &le) {
		return le
	}
	return nil
}
