// Package cdp hosts the vendored Chrome DevTools Protocol JSON definitions
// (cdp/protocol/) and the policy overlay manifest (cdp/manifest.yaml) that
// drive code generation of MCP tools in later phases. P1 only verifies the
// vendored artifacts parse and the domains the manifest skeleton names are
// present in the upstream definitions.
package cdp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type protocolFile struct {
	Version struct {
		Major string `json:"major"`
		Minor string `json:"minor"`
	} `json:"version"`
	Domains []struct {
		Domain       string `json:"domain"`
		Experimental bool   `json:"experimental,omitempty"`
	} `json:"domains"`
}

func loadProtocol(t *testing.T, name string) protocolFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("protocol", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var p protocolFile
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	if p.Version.Major == "" || len(p.Domains) == 0 {
		t.Fatalf("%s: parsed empty (version=%+v domains=%d)", name, p.Version, len(p.Domains))
	}
	return p
}

func TestProtocolJSONParses(t *testing.T) {
	loadProtocol(t, "browser_protocol.json")
	loadProtocol(t, "js_protocol.json")
}

// TestSkeletonDomainsPresent guards against a future protocol roll silently
// dropping a domain the P1 manifest skeleton names.
func TestSkeletonDomainsPresent(t *testing.T) {
	want := map[string]bool{
		"Browser": false,
		"Page":    false,
		"Runtime": false,
	}
	for _, name := range []string{"browser_protocol.json", "js_protocol.json"} {
		for _, d := range loadProtocol(t, name).Domains {
			if _, ok := want[d.Domain]; ok {
				want[d.Domain] = true
			}
		}
	}
	for d, found := range want {
		if !found {
			t.Errorf("manifest skeleton names %q but no protocol file defines it", d)
		}
	}
}

// TestVersionFile pins the vendored SHA so accidental refreshes need a
// deliberate edit. The body is checked verbatim against the source URL on
// roll; if you're rolling the protocol, update this constant in the same
// commit as cdp/protocol/VERSION.
func TestVersionFile(t *testing.T) {
	const wantSHA = "470fb6a42cbcaf446b516d8fc7738f9723cba5fc"
	raw, err := os.ReadFile(filepath.Join("protocol", "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	if !contains(string(raw), wantSHA) {
		t.Fatalf("VERSION missing pinned SHA %s", wantSHA)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
