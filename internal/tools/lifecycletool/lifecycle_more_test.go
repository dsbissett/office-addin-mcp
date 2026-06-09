package lifecycletool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/launch"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// validProjectDir writes a minimal detectable add-in project (package.json +
// XML manifest) into a fresh temp dir and returns the dir and the manifest
// path. Reuses the shared writeFile helper from lifecycle_test.go.
func validProjectDir(t *testing.T) (dir, manifestPath string) {
	t.Helper()
	dir = t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"name":"x"}`)
	manifestPath = filepath.Join(dir, "manifest.xml")
	writeFile(t, manifestPath,
		`<OfficeApp><Hosts><Host Name="Workbook"/></Hosts></OfficeApp>`)
	return dir, manifestPath
}

// --- runDetect ------------------------------------------------------------

func TestDetectTool_BadParams(t *testing.T) {
	res := Detect().Run(context.Background(), json.RawMessage(`{"cwd":123}`), &tools.RunEnv{})
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category = %q, want validation", res.Err.Category)
	}
}

// TestDetectTool_DetectFailed exercises the non-ErrNoProject branch: a
// package.json that exists but is malformed makes DetectAddin return a parse
// error rather than ErrNoProject, which the tool maps to detect_failed.
func TestDetectTool_DetectFailed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{not valid json`)

	raw, err := json.Marshal(map[string]string{"cwd": dir})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res := Detect().Run(context.Background(), raw, &tools.RunEnv{})
	if res.Err == nil || res.Err.Code != "detect_failed" {
		t.Fatalf("want detect_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryInternal {
		t.Errorf("category = %q, want internal", res.Err.Category)
	}
}

// --- runStop --------------------------------------------------------------

func TestStopTool_BadParams(t *testing.T) {
	res := Stop().Run(context.Background(), json.RawMessage(`{"all":"nope"}`), &tools.RunEnv{})
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category = %q, want validation", res.Err.Category)
	}
}

// TestStopTool_AllClearsManifest verifies the all=true path clears the active
// manifest through SetManifest and reports the count of tracked launches.
func TestStopTool_AllClearsManifest(t *testing.T) {
	cleared := false
	env := &tools.RunEnv{
		SetManifest: func(m *addin.Manifest) {
			if m == nil {
				cleared = true
			}
		},
	}
	raw, err := json.Marshal(map[string]bool{"all": true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res := Stop().Run(context.Background(), raw, env)
	if res.Err != nil {
		t.Fatalf("Stop all failed: %+v", res.Err)
	}
	if !cleared {
		t.Error("SetManifest(nil) not called on all=true")
	}
	body, err := json.Marshal(res.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	if !contains(string(body), `"all":true`) {
		t.Errorf("expected all=true in data, got %s", body)
	}
}

// TestStopTool_ManifestPathNotTracked passes an explicit manifestPath that the
// registry has never seen, hitting the "No tracked launch matched" no-op
// branch without going through cwd detection.
func TestStopTool_ManifestPathNotTracked(t *testing.T) {
	raw, err := json.Marshal(map[string]string{"manifestPath": "C:/nope/manifest.xml"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res := Stop().Run(context.Background(), raw, &tools.RunEnv{})
	if res.Err != nil {
		t.Fatalf("expected no-op success, got %+v", res.Err)
	}
	body, err := json.Marshal(res.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	if !contains(string(body), `"stopped":0`) {
		t.Errorf("expected stopped=0, got %s", body)
	}
	if !contains(string(body), `C:/nope/manifest.xml`) {
		t.Errorf("expected manifestPath echoed, got %s", body)
	}
}

// TestStopTool_DetectThenNotTracked drives the cwd-detection path: a valid
// project is detected so manifestPath is resolved from it, but no launch is
// tracked, so the tool reports the no-op success. This covers the
// DetectAddin-success arm of runStop that the manifestPath-only test skips.
func TestStopTool_DetectThenNotTracked(t *testing.T) {
	dir, manifestPath := validProjectDir(t)
	// Guard against a stray tracked launch from another test in this process.
	if _, ok := launch.LookupLaunch(manifestPath); ok {
		t.Skipf("manifest %s unexpectedly tracked", manifestPath)
	}
	raw, err := json.Marshal(map[string]string{"cwd": dir})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res := Stop().Run(context.Background(), raw, &tools.RunEnv{})
	if res.Err != nil {
		t.Fatalf("expected no-op success, got %+v", res.Err)
	}
	body, err := json.Marshal(res.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	if !contains(string(body), `"stopped":0`) {
		t.Errorf("expected stopped=0, got %s", body)
	}
}

// TestStopTool_DetectFailsForCwd verifies the addin_not_found arm of runStop
// when an explicit cwd has no project and no manifestPath is supplied.
func TestStopTool_DetectFailsForCwd(t *testing.T) {
	dir := t.TempDir()
	raw, err := json.Marshal(map[string]string{"cwd": dir})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res := Stop().Run(context.Background(), raw, &tools.RunEnv{})
	if res.Err == nil || res.Err.Code != "addin_not_found" {
		t.Fatalf("want addin_not_found, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryNotFound {
		t.Errorf("category = %q, want not_found", res.Err.Category)
	}
	if res.Err.Details == nil || res.Err.Details["cwd"] != dir {
		t.Errorf("expected cwd detail %q, got %+v", dir, res.Err.Details)
	}
}

// --- runLaunch ------------------------------------------------------------

func TestLaunchTool_BadParams(t *testing.T) {
	res := Launch().Run(context.Background(), json.RawMessage(`{"port":"x"}`), &tools.RunEnv{})
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category = %q, want validation", res.Err.Category)
	}
}

// TestLaunchTool_AddinNotFound drives runLaunch through DetectAddin failure for
// an explicit empty cwd. This also exercises the env.Logf / env.ReportProgress
// progress sinks (wired here) before detection fails.
func TestLaunchTool_AddinNotFound(t *testing.T) {
	dir := t.TempDir()
	logged := 0
	progressed := 0
	env := &tools.RunEnv{
		Log:      func(string, string) { logged++ },
		Progress: func(float64, float64, string) { progressed++ },
	}
	raw, err := json.Marshal(map[string]string{"cwd": dir})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res := Launch().Run(context.Background(), raw, env)
	if res.Err == nil || res.Err.Code != "addin_not_found" {
		t.Fatalf("want addin_not_found, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryNotFound {
		t.Errorf("category = %q, want not_found", res.Err.Category)
	}
	if res.Err.Details == nil || res.Err.Details["cwd"] != dir {
		t.Errorf("expected cwd detail %q, got %+v", dir, res.Err.Details)
	}
	if logged == 0 {
		t.Error("expected env.Logf to be invoked before detection")
	}
	if progressed == 0 {
		t.Error("expected env.ReportProgress to be invoked before detection")
	}
}

// TestLaunchTool_PortAlreadyConfigured drives runLaunch past detection into
// the real LaunchExcel, which fails deterministically (no process spawn) when
// WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS already declares a remote debugging
// port. This is the only way to exercise the launchErrToResult call site of
// runLaunch without spawning Excel. resolveLauncher (npx LookPath) and
// buildLaunchEnv both run before any child process is started.
func TestLaunchTool_PortAlreadyConfigured(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("runLaunch reaches launchErrToResult only past the Windows-only platform guard")
	}
	t.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", "--remote-debugging-port=9222")
	dir, manifestPath := validProjectDir(t)
	if _, ok := launch.LookupLaunch(manifestPath); ok {
		t.Skipf("manifest %s unexpectedly tracked", manifestPath)
	}
	env := &tools.RunEnv{
		Log:      func(string, string) {},
		Progress: func(float64, float64, string) {},
	}
	raw, err := json.Marshal(map[string]string{"cwd": dir})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res := Launch().Run(context.Background(), raw, env)
	if res.Err == nil {
		t.Fatalf("expected launch failure, got data %+v", res.Data)
	}
	// Either path is a no-spawn deterministic failure: with npx on PATH the
	// launcher resolves and buildLaunchEnv rejects the pre-set port; without it
	// resolveLauncher reports the launcher missing. Both flow through
	// launchErrToResult and yield the unsupported category.
	wantPort := "launch_" + launch.ReasonPortAlreadyConfig
	wantMissing := "launch_" + launch.ReasonLauncherMissing
	if res.Err.Code != wantPort && res.Err.Code != wantMissing {
		t.Fatalf("code = %q, want %q or %q", res.Err.Code, wantPort, wantMissing)
	}
	if res.Err.Category != tools.CategoryUnsupported {
		t.Errorf("category = %q, want unsupported", res.Err.Category)
	}
}

// --- empty-cwd / os.Getwd success path ------------------------------------

// The following three tests leave cwd empty so each tool falls back to
// os.Getwd(). The test binary's working directory (the package dir) has no
// add-in project within the upward walk, so detection deterministically fails
// with addin_not_found — without spawning anything — while still covering the
// os.Getwd() success branch in each runner.

func TestDetectTool_DefaultCwd(t *testing.T) {
	res := Detect().Run(context.Background(), json.RawMessage(`{}`), &tools.RunEnv{})
	if res.Err == nil || res.Err.Code != "addin_not_found" {
		t.Fatalf("want addin_not_found from default cwd, got %+v", res.Err)
	}
}

func TestStopTool_DefaultCwd(t *testing.T) {
	res := Stop().Run(context.Background(), json.RawMessage(`{}`), &tools.RunEnv{})
	if res.Err == nil || res.Err.Code != "addin_not_found" {
		t.Fatalf("want addin_not_found from default cwd, got %+v", res.Err)
	}
}

func TestLaunchTool_DefaultCwd(t *testing.T) {
	env := &tools.RunEnv{
		Log:      func(string, string) {},
		Progress: func(float64, float64, string) {},
	}
	res := Launch().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "addin_not_found" {
		t.Fatalf("want addin_not_found from default cwd, got %+v", res.Err)
	}
}

// --- launchErrToResult (pure mapper) --------------------------------------

func TestLaunchErrToResult_NonLaunchError(t *testing.T) {
	res := launchErrToResult(context.DeadlineExceeded)
	if res.Err == nil || res.Err.Code != "launch_failed" {
		t.Fatalf("want launch_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryInternal {
		t.Errorf("category = %q, want internal", res.Err.Category)
	}
	if res.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestLaunchErrToResult_Categories(t *testing.T) {
	cases := []struct {
		name      string
		reason    string
		wantCat   string
		wantCode  string
		retryable bool
	}{
		{"unsupported", launch.ReasonUnsupportedPlatform, tools.CategoryUnsupported, "launch_" + launch.ReasonUnsupportedPlatform, false},
		{"launcherMissing", launch.ReasonLauncherMissing, tools.CategoryUnsupported, "launch_" + launch.ReasonLauncherMissing, false},
		{"portConfigured", launch.ReasonPortAlreadyConfig, tools.CategoryUnsupported, "launch_" + launch.ReasonPortAlreadyConfig, false},
		{"cdpNotReady", launch.ReasonCDPNotReady, tools.CategoryTimeout, "launch_" + launch.ReasonCDPNotReady, true},
		{"devServerNotReady", launch.ReasonDevServerNotReady, tools.CategoryTimeout, "launch_" + launch.ReasonDevServerNotReady, true},
		{"genericFailed", launch.ReasonLaunchFailed, tools.CategoryInternal, "launch_" + launch.ReasonLaunchFailed, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			le := &launch.LaunchError{Reason: tc.reason, Message: "boom", Output: []string{"line1", "line2"}}
			res := launchErrToResult(le)
			if res.Err == nil {
				t.Fatalf("nil error for reason %q", tc.reason)
			}
			if res.Err.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", res.Err.Code, tc.wantCode)
			}
			if res.Err.Category != tc.wantCat {
				t.Errorf("category = %q, want %q", res.Err.Category, tc.wantCat)
			}
			if res.Err.Retryable != tc.retryable {
				t.Errorf("retryable = %v, want %v", res.Err.Retryable, tc.retryable)
			}
			if res.Err.Details["reason"] != tc.reason {
				t.Errorf("details.reason = %v, want %q", res.Err.Details["reason"], tc.reason)
			}
			out, ok := res.Err.Details["output"].([]string)
			if !ok || len(out) != 2 {
				t.Errorf("details.output = %v, want 2 captured lines", res.Err.Details["output"])
			}
			if res.Summary != "Launch failed: boom" {
				t.Errorf("summary = %q", res.Summary)
			}
		})
	}
}

// TestLaunchErrToResult_NoOutput covers the branch where the LaunchError has no
// captured output so the details map omits the "output" key.
func TestLaunchErrToResult_NoOutput(t *testing.T) {
	le := &launch.LaunchError{Reason: launch.ReasonAborted, Message: "stopped"}
	res := launchErrToResult(le)
	if res.Err == nil {
		t.Fatal("nil error")
	}
	if _, present := res.Err.Details["output"]; present {
		t.Errorf("output key should be absent when no output captured: %+v", res.Err.Details)
	}
	// ReasonAborted is not in the switch, so it stays internal/non-retryable.
	if res.Err.Category != tools.CategoryInternal {
		t.Errorf("category = %q, want internal", res.Err.Category)
	}
	if res.Err.Retryable {
		t.Error("aborted should not be retryable")
	}
}

// --- annotations ----------------------------------------------------------

// TestToolAnnotations asserts each tool carries the MCP hints expected from its
// behavior: addin.detect is read-only, addin.launch is an additive mutation,
// and addin.stop is a destructive one.
func TestToolAnnotations(t *testing.T) {
	t.Run("detect_is_readonly", func(t *testing.T) {
		a := Detect().Annotations
		if a == nil {
			t.Fatal("detect has no annotations")
		}
		if !a.ReadOnlyHint {
			t.Error("detect should be ReadOnlyHint=true")
		}
		if !a.IdempotentHint {
			t.Error("detect should be IdempotentHint=true")
		}
		if a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("detect should be DestructiveHint=false, got %v", a.DestructiveHint)
		}
	})

	t.Run("launch_is_additive_mutation", func(t *testing.T) {
		a := Launch().Annotations
		if a == nil {
			t.Fatal("launch has no annotations")
		}
		if a.ReadOnlyHint {
			t.Error("launch should not be ReadOnlyHint")
		}
		if a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("launch should be DestructiveHint=false, got %v", a.DestructiveHint)
		}
	})

	t.Run("stop_is_destructive", func(t *testing.T) {
		a := Stop().Annotations
		if a == nil {
			t.Fatal("stop has no annotations")
		}
		if a.ReadOnlyHint {
			t.Error("stop should not be ReadOnlyHint")
		}
		if a.DestructiveHint == nil || !*a.DestructiveHint {
			t.Errorf("stop should be DestructiveHint=true, got %v", a.DestructiveHint)
		}
	})
}

// --- codeFromReason (pure) ------------------------------------------------

func TestCodeFromReason(t *testing.T) {
	if got := codeFromReason(""); got != "launch_failed" {
		t.Errorf("empty reason: got %q, want launch_failed", got)
	}
	if got := codeFromReason(launch.ReasonCDPNotReady); got != "launch_"+launch.ReasonCDPNotReady {
		t.Errorf("got %q, want launch_%s", got, launch.ReasonCDPNotReady)
	}
}
