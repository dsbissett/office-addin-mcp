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
}

// LaunchResult is what the caller gets back after a successful sideload.
type LaunchResult struct {
	PID           int      `json:"pid"`
	CDPURL        string   `json:"cdpUrl"`
	ManifestPath  string   `json:"manifestPath"`
	DevServerPort int      `json:"devServerPort,omitempty"`
	Output        []string `json:"output,omitempty"`
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
	if existing, ok := defaultRegistry.lookup(project.ManifestPath); ok {
		return &LaunchResult{
			PID:          existing.PID,
			CDPURL:       existing.CDPURL,
			ManifestPath: project.ManifestPath,
		}, nil
	}

	port := opts.Port
	if port <= 0 {
		port = defaultCDPPort
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultLaunchTimeout
	}
	cdpURL := fmt.Sprintf("http://localhost:%d", port)

	launcherCmd, err := resolveLauncher(project.Root)
	if err != nil {
		return nil, &LaunchError{Reason: ReasonLauncherMissing, Message: err.Error()}
	}

	env, err := buildLaunchEnv(project.Root, port)
	if err != nil {
		return nil, err
	}

	var devServer *devServerHandle
	if !opts.SkipDevServer {
		devServer, err = ensureDevServer(ctx, project, env, opts.DevServerTimeout)
		if err != nil {
			return nil, &LaunchError{Reason: ReasonDevServerNotReady, Message: err.Error()}
		}
	}

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
	pid := cmd.Process.Pid
	exited := waitChild(cmd)

	if err := waitForCDPReady(ctx, cdpURL, timeout, exited, output); err != nil {
		killProcess(cmd)
		devServer.stop()
		return nil, err
	}

	tracked := &TrackedLaunch{
		Project:   project,
		CDPURL:    cdpURL,
		PID:       pid,
		Launcher:  launcherCmd,
		devServer: devServer,
	}
	tracked.StopFn = func() error {
		err := stopWithLauncher(launcherCmd, project, env)
		killProcess(cmd)
		devServer.stop()
		defaultRegistry.delete(project.ManifestPath)
		return err
	}
	defaultRegistry.put(project.ManifestPath, tracked)

	res := &LaunchResult{
		PID:          pid,
		CDPURL:       cdpURL,
		ManifestPath: project.ManifestPath,
		Output:       output.snapshot(),
	}
	if devServer != nil {
		res.DevServerPort = devServer.port
	}
	return res, nil
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
	out := make([]string, 0, len(pairs)+2)
	pathSet := false
	argsSet := false
	for _, kv := range pairs {
		key, _, ok := splitEnv(kv)
		if !ok {
			out = append(out, kv)
			continue
		}
		switch {
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
	if !pathSet {
		out = append(out, "PATH="+binDir)
	}
	if !argsSet {
		out = append(out, envWebView2ExtraArgs+"="+browserArgs)
	}
	return out, nil
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
		}
		probe := ProbeCDPEndpoint(ctx, cdpURL, cdpProbeTimeout)
		if probe.OK {
			return nil
		}
		lastReason = probe.Reason
		select {
		case <-time.After(probeInterval):
		case <-ctx.Done():
			return &LaunchError{Reason: ReasonAborted, Message: "launch aborted: " + ctx.Err().Error(), Output: output.snapshot()}
		}
	}
	return &LaunchError{
		Reason:  ReasonCDPNotReady,
		Message: fmt.Sprintf("timed out waiting for %s/json/version (%s)", cdpURL, lastReason),
		Output:  output.snapshot(),
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
		if err != nil {
			return &LaunchError{Reason: ReasonStopFailed, Message: fmt.Sprintf("%s stop: %v", launcherToolName, err), Output: output.snapshot()}
		}
		return nil
	case <-time.After(stopTimeout):
		killProcess(cmd)
		return &LaunchError{Reason: ReasonStopFailed, Message: fmt.Sprintf("timed out waiting for %s stop", launcherToolName), Output: output.snapshot()}
	}
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
