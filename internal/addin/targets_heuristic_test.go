package addin

import (
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

func TestHeuristicSurface_AllBranches(t *testing.T) {
	cases := map[string]SurfaceType{
		"":                                   "",
		"about:blank":                        "",
		"devtools://devtools/inspector.html": "",
		"chrome://version":                   "",
		"edge://settings":                    "",
		"https://x/Dialog.html":              SurfaceDialog, // case-insensitive "dialog"
		"https://x/functions.html":           SurfaceCFRuntime,
		"https://x/functions/index.html":     SurfaceCFRuntime, // "/functions/" path segment
		"https://x/taskpane.html":            SurfaceTaskpane,  // generic http(s)
		"http://x/taskpane.html":             SurfaceTaskpane,
		"ftp://x/file":                       "", // non-http(s), no keyword -> default empty
		"data:text/html,abc":                 "", // unknown scheme -> default empty
	}
	for in, want := range cases {
		if got := heuristicSurface(in); got != want {
			t.Errorf("heuristicSurface(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClassifyTargets_EmptyPatternSkipped(t *testing.T) {
	// A surface with an empty pattern must be skipped, so the URL falls through
	// to the heuristic classifier rather than matching everything.
	m := &Manifest{
		ID: "x",
		Surfaces: []Surface{
			{Type: SurfaceContent, URL: "https://content/", Pattern: ""},
		},
	}
	targets := []cdp.TargetInfo{
		{TargetID: "a", URL: "https://anything/taskpane.html"},
	}
	out := ClassifyTargets(targets, m)
	if len(out) != 1 || out[0].Surface != SurfaceTaskpane {
		t.Fatalf("got %+v, want one taskpane (heuristic) target", out)
	}
	if out[0].MatchedURL != "" {
		t.Errorf("MatchedURL should be empty when no pattern matched, got %q", out[0].MatchedURL)
	}
}

func TestClassifyTargets_LongestPatternWins(t *testing.T) {
	// A specific path pattern must win over the bare-host pattern even when
	// both substrings appear in the target URL.
	m := &Manifest{
		ID: "x",
		Surfaces: []Surface{
			{Type: SurfaceContent, URL: "https://host/", Pattern: "host"},
			{Type: SurfaceDialog, URL: "https://host/path/dialog.html", Pattern: "host/path/dialog.html"},
		},
	}
	targets := []cdp.TargetInfo{
		{TargetID: "d", URL: "https://host/path/dialog.html?q=1"},
	}
	out := ClassifyTargets(targets, m)
	if len(out) != 1 || out[0].Surface != SurfaceDialog {
		t.Fatalf("got %+v, want dialog (longest pattern wins)", out)
	}
	if out[0].MatchedURL != "https://host/path/dialog.html" {
		t.Errorf("MatchedURL = %q", out[0].MatchedURL)
	}
}

func TestClassifyTargets_StampsIdentity(t *testing.T) {
	m := &Manifest{ID: "the-id", DisplayName: "The Name"}
	out := ClassifyTargets([]cdp.TargetInfo{{TargetID: "t", URL: "about:blank"}}, m)
	if len(out) != 1 {
		t.Fatalf("len = %d", len(out))
	}
	if out[0].AddinID != "the-id" || out[0].DisplayName != "The Name" {
		t.Errorf("identity not stamped: %+v", out[0])
	}
}

func TestClassifyTargets_Empty(t *testing.T) {
	out := ClassifyTargets(nil, nil)
	if len(out) != 0 {
		t.Errorf("expected empty result, got %v", out)
	}
}
