package lifecycletool

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/launch"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// fallbackManifestJSON is a minimal manifest addin.ParseManifest accepts (mirrors
// internal/addin/testdata/hostfallback.json) so the SetManifest success branch
// of applyLaunchSuccess can be exercised.
const fallbackManifestJSON = `{
  "id": "host-fallback-id",
  "name": { "short": "OnlyShort" },
  "extensions": [{ "requirements": { "scopes": [], "capabilities": [] }, "runtimes": [] }],
  "host": "Mailbox"
}`

func TestApplyLaunchSuccess_AllHooksWithManifest(t *testing.T) {
	dir := t.TempDir()
	mpath := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(mpath, []byte(fallbackManifestJSON), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	var gotEndpoint webview2.Config
	reset := false
	var gotManifest *addin.Manifest
	env := &tools.RunEnv{
		SetEndpoint:   func(c webview2.Config) { gotEndpoint = c },
		ResetSessions: func() { reset = true },
		SetManifest:   func(m *addin.Manifest) { gotManifest = m },
	}

	applyLaunchSuccess(env, &launch.LaunchResult{CDPURL: "http://127.0.0.1:9222", ManifestPath: mpath})

	if gotEndpoint.BrowserURL != "http://127.0.0.1:9222" {
		t.Errorf("SetEndpoint got %q", gotEndpoint.BrowserURL)
	}
	if !reset {
		t.Error("ResetSessions not called")
	}
	if gotManifest == nil {
		t.Error("SetManifest not called with parsed manifest")
	}
}

func TestApplyLaunchSuccess_InvalidManifestSkipsSet(t *testing.T) {
	called := false
	env := &tools.RunEnv{
		SetManifest: func(*addin.Manifest) { called = true },
	}
	applyLaunchSuccess(env, &launch.LaunchResult{CDPURL: "x", ManifestPath: filepath.Join(t.TempDir(), "missing.xml")})
	if called {
		t.Error("SetManifest should not fire when ParseManifest fails")
	}
}

func TestApplyLaunchSuccess_NilHooksNoPanic(t *testing.T) {
	applyLaunchSuccess(&tools.RunEnv{}, &launch.LaunchResult{CDPURL: "x"})
}

func TestLaunchSummary(t *testing.T) {
	withDev := launchSummary(&launch.LaunchResult{PID: 42, CDPURL: "http://h:9222", DevServerPort: 3000})
	if !contains(withDev, "dev server on :3000") || !contains(withDev, "pid=42") {
		t.Errorf("with-dev summary = %q", withDev)
	}
	noDev := launchSummary(&launch.LaunchResult{PID: 7, CDPURL: "http://h:9222"})
	if contains(noDev, "dev server") || !contains(noDev, "pid=7") {
		t.Errorf("no-dev summary = %q", noDev)
	}
}

func TestStopExcelErrToResult(t *testing.T) {
	le := &launch.LaunchError{Reason: "cdp_not_ready", Message: "no cdp", Output: []string{"line1"}}
	res := stopExcelErrToResult(le, "/tmp/m.xml")
	if res.Err == nil || res.Err.Code != "stop_failed" {
		t.Fatalf("want stop_failed, got %+v", res.Err)
	}
	if res.Err.Details["reason"] != "cdp_not_ready" {
		t.Errorf("reason = %v", res.Err.Details["reason"])
	}
	if res.Err.Details["output"] == nil {
		t.Error("output not attached")
	}

	plain := stopExcelErrToResult(errors.New("boom"), "/tmp/m.xml")
	if plain.Err.Details["reason"] != nil {
		t.Error("plain error should have no reason detail")
	}
	if plain.Err.Details["manifestPath"] != "/tmp/m.xml" {
		t.Errorf("manifestPath = %v", plain.Err.Details["manifestPath"])
	}
}

func TestClearActiveManifest(t *testing.T) {
	const mp = "/tmp/active.xml"

	// Matching active manifest -> SetManifest(nil) is called.
	cleared := false
	env := &tools.RunEnv{
		SetManifest: func(m *addin.Manifest) {
			if m != nil {
				t.Errorf("expected nil manifest, got %+v", m)
			}
			cleared = true
		},
		Manifest: func() *addin.Manifest { return &addin.Manifest{Path: mp} },
	}
	clearActiveManifest(env, mp)
	if !cleared {
		t.Error("matching manifest should be cleared")
	}

	// Non-matching active manifest -> SetManifest is NOT called.
	called := false
	env2 := &tools.RunEnv{
		SetManifest: func(*addin.Manifest) { called = true },
		Manifest:    func() *addin.Manifest { return &addin.Manifest{Path: "/other.xml"} },
	}
	clearActiveManifest(env2, mp)
	if called {
		t.Error("non-matching manifest must not be cleared")
	}
}

func TestActiveManifestMatches_Guards(t *testing.T) {
	cases := []struct {
		name string
		env  *tools.RunEnv
		want bool
	}{
		{"nil env", nil, false},
		{"nil SetManifest", &tools.RunEnv{Manifest: func() *addin.Manifest { return &addin.Manifest{Path: "/m"} }}, false},
		{"nil Manifest", &tools.RunEnv{SetManifest: func(*addin.Manifest) {}}, false},
		{"current nil", &tools.RunEnv{SetManifest: func(*addin.Manifest) {}, Manifest: func() *addin.Manifest { return nil }}, false},
		{"match", &tools.RunEnv{SetManifest: func(*addin.Manifest) {}, Manifest: func() *addin.Manifest { return &addin.Manifest{Path: "/m"} }}, true},
	}
	for _, tc := range cases {
		if got := activeManifestMatches(tc.env, "/m"); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
