package launch

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// --- decodePortValue ------------------------------------------------------

func TestDecodePortValue(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		want   int
		wantOK bool
	}{
		{"empty", "", 0, false},
		{"number", "3000", 3000, true},
		{"zero-number", "0", 0, false},
		{"negative-number", "-5", 0, false},
		{"numeric-string", `"8080"`, 8080, true},
		{"zero-string", `"0"`, 0, false},
		{"empty-string", `""`, 0, false},
		{"garbage-string", `"abc"`, 0, false},
		{"object", `{"a":1}`, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw json.RawMessage
			if tc.raw != "" {
				raw = json.RawMessage(tc.raw)
			}
			got, ok := decodePortValue(raw)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("decodePortValue(%s) = (%d,%v), want (%d,%v)", tc.raw, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// --- detectDevServer ------------------------------------------------------

func TestDetectDevServer(t *testing.T) {
	t.Run("nil pkg", func(t *testing.T) {
		if got := detectDevServer(nil); got != nil {
			t.Errorf("detectDevServer(nil) = %+v, want nil", got)
		}
	})
	t.Run("no scripts", func(t *testing.T) {
		if got := detectDevServer(&packageJSON{}); got != nil {
			t.Errorf("detectDevServer(empty) = %+v, want nil", got)
		}
	})
	t.Run("no matching script name", func(t *testing.T) {
		pkg := &packageJSON{Scripts: map[string]string{"build": "webpack"}}
		if got := detectDevServer(pkg); got != nil {
			t.Errorf("detectDevServer(build-only) = %+v, want nil", got)
		}
	})
	t.Run("empty script value is skipped", func(t *testing.T) {
		pkg := &packageJSON{Scripts: map[string]string{"dev-server": ""}}
		if got := detectDevServer(pkg); got != nil {
			t.Errorf("detectDevServer(empty dev-server) = %+v, want nil", got)
		}
	})
	t.Run("dev:server name with port", func(t *testing.T) {
		pkg := &packageJSON{Scripts: map[string]string{"dev:server": "webpack serve"}}
		pkg.Config.DevServerPort = json.RawMessage(`3001`)
		got := detectDevServer(pkg)
		if got == nil || got.Script != "dev:server" || got.Port != 3001 {
			t.Errorf("detectDevServer = %+v, want {dev:server, 3001}", got)
		}
	})
	t.Run("matching script but no usable port", func(t *testing.T) {
		pkg := &packageJSON{Scripts: map[string]string{"dev": "vite"}}
		// No config.dev_server_port -> decodePortValue fails -> nil.
		if got := detectDevServer(pkg); got != nil {
			t.Errorf("detectDevServer(no port) = %+v, want nil", got)
		}
	})
}

// --- detectPackageManager -------------------------------------------------

func TestDetectPackageManager(t *testing.T) {
	t.Run("yarn lock", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "yarn.lock"), "# yarn lockfile v1\n")
		if got := detectPackageManager(dir); got != PackageManagerYarn {
			t.Errorf("detectPackageManager = %s, want yarn", got)
		}
	})
	t.Run("pnpm wins over default", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "pnpm-lock.yaml"), "lockfileVersion: 6.0\n")
		if got := detectPackageManager(dir); got != PackageManagerPnpm {
			t.Errorf("detectPackageManager = %s, want pnpm", got)
		}
	})
	t.Run("default npm", func(t *testing.T) {
		dir := t.TempDir()
		if got := detectPackageManager(dir); got != PackageManagerNpm {
			t.Errorf("detectPackageManager = %s, want npm", got)
		}
	})
}

// --- isOfficeXMLManifest / isOfficeJSONManifest negative paths ------------

func TestManifestPredicates_Negatives(t *testing.T) {
	dir := t.TempDir()

	missing := filepath.Join(dir, "nope.xml")
	if isOfficeXMLManifest(missing) {
		t.Error("isOfficeXMLManifest(missing) = true, want false")
	}
	if isOfficeJSONManifest(missing) {
		t.Error("isOfficeJSONManifest(missing) = true, want false")
	}

	notOffice := filepath.Join(dir, "plain.xml")
	writeFile(t, notOffice, `<root><child/></root>`)
	if isOfficeXMLManifest(notOffice) {
		t.Error("isOfficeXMLManifest(non-office xml) = true, want false")
	}

	badJSON := filepath.Join(dir, "bad.json")
	writeFile(t, badJSON, `{ not valid json`)
	if isOfficeJSONManifest(badJSON) {
		t.Error("isOfficeJSONManifest(malformed) = true, want false")
	}

	emptyScopes := filepath.Join(dir, "empty.json")
	writeFile(t, emptyScopes, `{"extensions":[{"requirements":{"scopes":[]}}]}`)
	if isOfficeJSONManifest(emptyScopes) {
		t.Error("isOfficeJSONManifest(empty scopes) = true, want false")
	}
}

// --- readPackageJSON error paths ------------------------------------------

func TestReadPackageJSON_Errors(t *testing.T) {
	if _, err := readPackageJSON(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("readPackageJSON(missing) = nil error, want read error")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	writeFile(t, bad, `{ broken`)
	if _, err := readPackageJSON(bad); err == nil {
		t.Error("readPackageJSON(malformed) = nil error, want parse error")
	}
}

// --- DetectAddin: package.json present but malformed ----------------------

func TestDetectAddin_MalformedPackageJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{ not json`)
	writeFile(t, filepath.Join(dir, "manifest.xml"), `<OfficeApp/>`)
	if _, err := DetectAddin(dir); err == nil {
		t.Fatal("DetectAddin: expected parse error for malformed package.json")
	}
}

// --- ProbeCDPEndpoint extra branches --------------------------------------

func TestProbeCDPEndpoint_InvalidRequestURL(t *testing.T) {
	// A control character in the URL makes http.NewRequestWithContext fail,
	// exercising the invalid-request branch.
	probe := ProbeCDPEndpoint(context.Background(), "http://exa\x7fmple", time.Second)
	if probe.OK {
		t.Fatal("probe.OK = true, want false for malformed URL")
	}
	if probe.Reason == "" {
		t.Error("Reason = empty, want invalid-request:...")
	}
}

func TestProbeCDPEndpoint_Timeout(t *testing.T) {
	// 10.255.255.1 is a non-routable address; the dial blocks until the tiny
	// timeout fires, hitting the DeadlineExceeded -> "timeout" branch.
	probe := ProbeCDPEndpoint(context.Background(), "http://10.255.255.1:9", 50*time.Millisecond)
	if probe.OK {
		t.Fatal("probe.OK = true, want false")
	}
	// Reason is either "timeout" or "unreachable" depending on the host's
	// network stack; both are valid non-OK outcomes.
	if probe.Reason == "" {
		t.Error("Reason = empty, want a non-OK reason")
	}
}
