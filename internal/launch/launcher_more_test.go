package launch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// --- LaunchError ----------------------------------------------------------

func TestLaunchError_Error(t *testing.T) {
	noOut := &LaunchError{Reason: ReasonLaunchFailed, Message: "boom"}
	if got := noOut.Error(); got != "boom" {
		t.Errorf("Error() = %q, want %q", got, "boom")
	}
	withOut := &LaunchError{Message: "boom", Output: []string{"l1", "l2"}}
	if got := withOut.Error(); got != "boom\nl1\nl2" {
		t.Errorf("Error() = %q, want boom\\nl1\\nl2", got)
	}
}

func TestAsLaunchError(t *testing.T) {
	le := &LaunchError{Reason: ReasonAborted, Message: "x"}
	if got := AsLaunchError(le); got != le {
		t.Errorf("AsLaunchError(le) = %v, want the same pointer", got)
	}
	wrapped := fmt.Errorf("context: %w", le)
	if got := AsLaunchError(wrapped); got != le {
		t.Errorf("AsLaunchError(wrapped) = %v, want unwrapped le", got)
	}
	if got := AsLaunchError(errors.New("plain")); got != nil {
		t.Errorf("AsLaunchError(plain) = %v, want nil", got)
	}
}

// --- splitEnv / envValue --------------------------------------------------

func TestSplitEnvAndEnvValue(t *testing.T) {
	k, v, ok := splitEnv("FOO=bar=baz")
	if !ok || k != "FOO" || v != "bar=baz" {
		t.Errorf("splitEnv = (%q,%q,%v), want (FOO, bar=baz, true)", k, v, ok)
	}
	if _, _, ok := splitEnv("NOEQUALS"); ok {
		t.Error("splitEnv(NOEQUALS) ok = true, want false")
	}
	if got := envValue("FOO=bar"); got != "bar" {
		t.Errorf("envValue = %q, want bar", got)
	}
	if got := envValue("noequals"); got != "" {
		t.Errorf("envValue(noequals) = %q, want empty", got)
	}
}

// --- buildLaunchEnv -------------------------------------------------------

func TestBuildLaunchEnv_InjectsPathAndArgs(t *testing.T) {
	// Ensure the conflicting env var is empty so we hit the happy path
	// (remoteDebugRE never matches an empty value).
	t.Setenv(envWebView2ExtraArgs, "")

	root := filepath.FromSlash("C:/proj")
	env, err := buildLaunchEnv(root, 9333)
	if err != nil {
		t.Fatalf("buildLaunchEnv: %v", err)
	}

	var sawPath, sawArgs bool
	wantArgs := fmt.Sprintf("%s=%d", envRemoteDebuggingArg, 9333)
	binDir := localBinDir(root)
	for _, kv := range env {
		key, val, ok := splitEnv(kv)
		if !ok {
			continue
		}
		if strings.EqualFold(key, "PATH") {
			sawPath = true
			if !strings.HasPrefix(val, binDir+string(os.PathListSeparator)) && val != binDir {
				t.Errorf("PATH = %q, want it to start with %q", val, binDir)
			}
		}
		if strings.EqualFold(key, envWebView2ExtraArgs) {
			sawArgs = true
			if val != wantArgs {
				t.Errorf("%s = %q, want %q", envWebView2ExtraArgs, val, wantArgs)
			}
		}
	}
	if !sawPath {
		t.Error("buildLaunchEnv did not set PATH")
	}
	if !sawArgs {
		t.Errorf("buildLaunchEnv did not set %s", envWebView2ExtraArgs)
	}
}

func TestBuildLaunchEnv_OverridesExistingWebView2Args(t *testing.T) {
	// Pre-set a benign (non remote-debugging) value: it must be replaced, not
	// appended, so we still see exactly one entry with the new value.
	t.Setenv(envWebView2ExtraArgs, "--something-else")
	env, err := buildLaunchEnv("C:/proj", 9444)
	if err != nil {
		t.Fatalf("buildLaunchEnv: %v", err)
	}
	want := fmt.Sprintf("%s=%d", envRemoteDebuggingArg, 9444)
	count := 0
	for _, kv := range env {
		key, val, ok := splitEnv(kv)
		if ok && strings.EqualFold(key, envWebView2ExtraArgs) {
			count++
			if val != want {
				t.Errorf("%s = %q, want %q", envWebView2ExtraArgs, val, want)
			}
		}
	}
	if count != 1 {
		t.Errorf("%s appeared %d times, want exactly 1", envWebView2ExtraArgs, count)
	}
}

func TestBuildLaunchEnv_RefusesWhenPortAlreadyConfigured(t *testing.T) {
	t.Setenv(envWebView2ExtraArgs, "--remote-debugging-port=9999")
	_, err := buildLaunchEnv("C:/proj", 9222)
	if err == nil {
		t.Fatal("buildLaunchEnv: expected error when port already configured")
	}
	le := AsLaunchError(err)
	if le == nil || le.Reason != ReasonPortAlreadyConfig {
		t.Fatalf("err = %v, want LaunchError{Reason: %q}", err, ReasonPortAlreadyConfig)
	}
}

func TestRemoteDebugRegex(t *testing.T) {
	cases := map[string]bool{
		"--remote-debugging-port=9222":  true,
		"foo --remote-debugging-port":   true,
		"--Remote-Debugging-Port 9222":  true, // case-insensitive
		"x --remote-debugging-port=1 y": true,
		"--remote-debugging-pipe":       false,
		"":                              false,
		"--other-flag --remote-debug":   false,
	}
	for in, want := range cases {
		if got := remoteDebugRE.MatchString(in); got != want {
			t.Errorf("remoteDebugRE.MatchString(%q) = %v, want %v", in, got, want)
		}
	}
}

// --- resolveLauncher ------------------------------------------------------

func TestResolveLauncher_PrefersLocalShim(t *testing.T) {
	root := t.TempDir()
	binDir := localBinDir(root)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Create a shim file. On Windows resolveLauncher checks .cmd first.
	shimName := launcherToolName + ".cmd"
	if runtime.GOOS != "windows" {
		shimName = launcherToolName // bare name candidate
	}
	shim := filepath.Join(binDir, shimName)
	if err := os.WriteFile(shim, []byte("@echo off\n"), 0o755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	got, err := resolveLauncher(root)
	if err != nil {
		t.Fatalf("resolveLauncher: %v", err)
	}
	if got != shim {
		t.Errorf("resolveLauncher = %q, want local shim %q", got, shim)
	}
}

func TestResolveLauncher_FallsBackToNpxOrErrors(t *testing.T) {
	root := t.TempDir() // no local shim
	got, err := resolveLauncher(root)
	if _, lookErr := lookPathNpx(); lookErr == nil {
		// npx is on PATH in this environment: resolveLauncher must return it.
		if err != nil {
			t.Fatalf("resolveLauncher: %v (expected npx fallback)", err)
		}
		if filepath.Base(got) != "npx" && filepath.Base(got) != "npx.exe" && filepath.Base(got) != "npx.cmd" {
			t.Errorf("resolveLauncher = %q, want an npx path", got)
		}
		return
	}
	// No npx: must report launcher-missing.
	if err == nil {
		t.Fatal("resolveLauncher: expected error when no shim and no npx")
	}
	if !errors.Is(err, errLauncherMissing) {
		t.Errorf("err = %v, want errLauncherMissing", err)
	}
}

// --- buildLauncherCommand -------------------------------------------------

func TestBuildLauncherCommand_LocalShim(t *testing.T) {
	proj := &Project{Root: "C:/proj", ManifestPath: "C:/proj/manifest.xml"}
	shim := filepath.FromSlash("C:/proj/node_modules/.bin/office-addin-debugging.cmd")
	cmd, err := buildLauncherCommand(shim, "start", proj, []string{"X=1"})
	if err != nil {
		t.Fatalf("buildLauncherCommand: %v", err)
	}
	if cmd.Dir != "C:/proj" {
		t.Errorf("cmd.Dir = %q, want C:/proj", cmd.Dir)
	}
	// A local shim is not npx, so no --no-install prefix: argv is exactly
	// [shim, start, manifest].
	if len(cmd.Args) != 3 {
		t.Fatalf("cmd.Args = %v, want 3 entries", cmd.Args)
	}
	if cmd.Args[1] != "start" || cmd.Args[2] != "C:/proj/manifest.xml" {
		t.Errorf("cmd.Args = %v, want [shim start manifest]", cmd.Args)
	}
}

func TestBuildLauncherCommand_NpxPrefixesNoInstall(t *testing.T) {
	proj := &Project{Root: "C:/proj", ManifestPath: "C:/proj/manifest.xml"}
	for _, npx := range []string{"npx", "npx.cmd", "npx.exe"} {
		launcher := filepath.Join("C:/tools", npx)
		cmd, err := buildLauncherCommand(launcher, "stop", proj, nil)
		if err != nil {
			t.Fatalf("buildLauncherCommand(%s): %v", npx, err)
		}
		// argv: [npx, --no-install, office-addin-debugging, stop, manifest]
		if len(cmd.Args) != 5 {
			t.Fatalf("cmd.Args(%s) = %v, want 5 entries", npx, cmd.Args)
		}
		if cmd.Args[1] != "--no-install" || cmd.Args[2] != launcherToolName {
			t.Errorf("cmd.Args(%s) = %v, want --no-install office-addin-debugging prefix", npx, cmd.Args)
		}
		if cmd.Args[3] != "stop" || cmd.Args[4] != "C:/proj/manifest.xml" {
			t.Errorf("cmd.Args(%s) tail = %v, want [... stop manifest]", npx, cmd.Args)
		}
	}
}

// --- waitForCDPReady ------------------------------------------------------

func TestWaitForCDPReady_OK(t *testing.T) {
	cdpURL := startCDPStub(t)
	exited := make(chan error, 1) // never fires
	out := newOutputBuffer(maxOutputLines)
	if err := waitForCDPReady(context.Background(), cdpURL, 5*time.Second, exited, out); err != nil {
		t.Fatalf("waitForCDPReady: %v", err)
	}
}

func TestWaitForCDPReady_ChildExitsEarly(t *testing.T) {
	// Nothing serving the port -> probe fails; child-exit fires first.
	port := freeClosedPort(t)
	cdpURL := fmt.Sprintf("http://localhost:%d", port)
	exited := make(chan error, 1)
	exited <- errors.New("exit status 1")
	out := newOutputBuffer(maxOutputLines)
	out.append([]byte("some launcher noise\n"))
	err := waitForCDPReady(context.Background(), cdpURL, 5*time.Second, exited, out)
	if err == nil {
		t.Fatal("waitForCDPReady: expected error when launcher exits early")
	}
	le := AsLaunchError(err)
	if le == nil || le.Reason != ReasonLaunchFailed {
		t.Fatalf("err = %v, want LaunchError{Reason: launch-failed}", err)
	}
	if len(le.Output) == 0 {
		t.Error("expected captured Output in the LaunchError")
	}
}

func TestWaitForCDPReady_ContextCancelled(t *testing.T) {
	port := freeClosedPort(t)
	cdpURL := fmt.Sprintf("http://localhost:%d", port)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled: first select hits ctx.Done()
	exited := make(chan error, 1)
	out := newOutputBuffer(maxOutputLines)
	err := waitForCDPReady(ctx, cdpURL, 5*time.Second, exited, out)
	le := AsLaunchError(err)
	if le == nil || le.Reason != ReasonAborted {
		t.Fatalf("err = %v, want LaunchError{Reason: aborted}", err)
	}
}

func TestWaitForCDPReady_Timeout(t *testing.T) {
	port := freeClosedPort(t)
	cdpURL := fmt.Sprintf("http://localhost:%d", port)
	exited := make(chan error, 1) // never fires
	out := newOutputBuffer(maxOutputLines)
	// Tiny timeout so the deadline loop exits without ever succeeding.
	err := waitForCDPReady(context.Background(), cdpURL, 50*time.Millisecond, exited, out)
	le := AsLaunchError(err)
	if le == nil || le.Reason != ReasonCDPNotReady {
		t.Fatalf("err = %v, want LaunchError{Reason: cdp-not-ready}", err)
	}
}

// --- StopExcel ------------------------------------------------------------

func TestStopExcel_NoTrackedLaunchIsNil(t *testing.T) {
	if err := StopExcel("C:/never/registered/manifest.xml"); err != nil {
		t.Errorf("StopExcel(unknown) = %v, want nil (idempotent)", err)
	}
}

func TestStopExcel_RunsTrackedStop(t *testing.T) {
	t.Cleanup(func() { defaultRegistry.launches = map[string]*TrackedLaunch{} })
	manifest := "C:/proj/manifest.xml"
	called := false
	tl := &TrackedLaunch{Project: &Project{ManifestPath: manifest}}
	tl.StopFn = func() error { called = true; return errors.New("stop-err") }
	defaultRegistry.put(manifest, tl)

	err := StopExcel(manifest)
	if !called {
		t.Error("StopExcel did not invoke the tracked StopFn")
	}
	if err == nil || err.Error() != "stop-err" {
		t.Errorf("StopExcel err = %v, want stop-err", err)
	}
}

// --- LaunchExcel branches (no real Office spawn) --------------------------

func TestLaunchExcel_ReusesAliveTrackedLaunch(t *testing.T) {
	t.Cleanup(func() { defaultRegistry.launches = map[string]*TrackedLaunch{} })
	cdpURL := startCDPStub(t)
	manifest := "C:/proj/reuse-manifest.xml"
	proj := &Project{ManifestPath: manifest, Root: t.TempDir()}

	tl := &TrackedLaunch{
		Project: proj,
		CDPURL:  cdpURL,
		PID:     os.Getpid(), // this test process is alive
	}
	defaultRegistry.put(manifest, tl)

	res, err := LaunchExcel(context.Background(), proj, LaunchOptions{})
	if err != nil {
		t.Fatalf("LaunchExcel: %v", err)
	}
	if res.Source != "reused" {
		t.Errorf("Source = %q, want reused", res.Source)
	}
	if !res.CDPVerified {
		t.Error("CDPVerified = false, want true on reuse")
	}
	if res.PID != os.Getpid() || res.CDPURL != cdpURL {
		t.Errorf("res = %+v, want PID/CDPURL from the tracked launch", res)
	}
}

func TestLaunchExcel_StaleRecordRelaunchHitsEnvGuard(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("LaunchExcel is Windows-only past the platform guard")
	}
	t.Cleanup(func() { defaultRegistry.launches = map[string]*TrackedLaunch{} })
	// Force buildLaunchEnv to fail fast (after resolveLauncher) so we exercise
	// the stale-record cleanup + relaunch path without spawning office tooling.
	t.Setenv(envWebView2ExtraArgs, "--remote-debugging-port=1")

	manifest := "C:/proj/stale-manifest.xml"
	proj := &Project{ManifestPath: manifest, Root: t.TempDir()}
	stopCalled := false
	tl := &TrackedLaunch{
		Project: proj,
		CDPURL:  "http://localhost:1", // probe will fail
		PID:     0,                    // dead -> stale
		StopFn:  func() error { stopCalled = true; return nil },
	}
	defaultRegistry.put(manifest, tl)

	_, err := LaunchExcel(context.Background(), proj, LaunchOptions{SkipDevServer: true})
	if err == nil {
		t.Fatal("LaunchExcel: expected error from the port-already-configured guard")
	}
	if !stopCalled {
		t.Error("stale record Stop() was not invoked before relaunch")
	}
	le := AsLaunchError(err)
	if le == nil || le.Reason != ReasonPortAlreadyConfig {
		t.Fatalf("err = %v, want port-already-configured", err)
	}
}

func TestLaunchExcel_LauncherMissing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("LaunchExcel is Windows-only past the platform guard")
	}
	if _, err := lookPathNpx(); err == nil {
		t.Skip("npx is on PATH; resolveLauncher cannot report launcher-missing here")
	}
	t.Cleanup(func() { defaultRegistry.launches = map[string]*TrackedLaunch{} })
	proj := &Project{ManifestPath: "C:/proj/none.xml", Root: t.TempDir()}
	_, err := LaunchExcel(context.Background(), proj, LaunchOptions{SkipDevServer: true})
	le := AsLaunchError(err)
	if le == nil || le.Reason != ReasonLauncherMissing {
		t.Fatalf("err = %v, want launcher-missing", err)
	}
}

// --- helpers --------------------------------------------------------------

// startCDPStub serves a minimal /json/version on a fresh localhost port and
// returns its http://localhost:PORT base URL. It binds 127.0.0.1 explicitly
// because the launch code probes localhost only.
func startCDPStub(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Browser":"CDPStub/1.0"}`))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return fmt.Sprintf("http://localhost:%d", port)
}

// lookPathNpx mirrors resolveLauncher's npx fallback probe so tests can decide
// whether the launcher-missing branch is reachable in this environment.
func lookPathNpx() (string, error) {
	return exec.LookPath("npx")
}
