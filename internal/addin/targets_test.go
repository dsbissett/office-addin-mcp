package addin

import (
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

func TestClassifyTargets_WithManifest(t *testing.T) {
	m := &Manifest{
		ID:          "abc",
		DisplayName: "Sample",
		Surfaces: []Surface{
			{Type: SurfaceTaskpane, URL: "https://localhost:3000/taskpane.html", Pattern: "localhost:3000/taskpane.html"},
			{Type: SurfaceCFRuntime, URL: "https://localhost:3000/functions.html", Pattern: "localhost:3000/functions.html"},
		},
	}
	targets := []cdp.TargetInfo{
		{TargetID: "t1", Type: "page", URL: "https://localhost:3000/taskpane.html"},
		{TargetID: "t2", Type: "page", URL: "https://localhost:3000/functions.html"},
		{TargetID: "t3", Type: "page", URL: "https://other.example.com/page"},
		{TargetID: "t4", Type: "page", URL: "devtools://devtools/inspector.html"},
	}
	out := ClassifyTargets(targets, m)
	if len(out) != 4 {
		t.Fatalf("len = %d, want 4", len(out))
	}
	wantSurface := map[string]SurfaceType{
		"t1": SurfaceTaskpane,
		"t2": SurfaceCFRuntime,
		"t3": SurfaceTaskpane, // heuristic falls back to taskpane for http(s)
		"t4": "",
	}
	for _, c := range out {
		if c.Surface != wantSurface[c.TargetID] {
			t.Errorf("target %s: surface = %q, want %q", c.TargetID, c.Surface, wantSurface[c.TargetID])
		}
		if c.Surface == SurfaceTaskpane && c.TargetID == "t1" && c.AddinID != "abc" {
			t.Errorf("AddinID not stamped: %+v", c)
		}
	}
}

func TestClassifyTargets_WithoutManifest(t *testing.T) {
	targets := []cdp.TargetInfo{
		{TargetID: "tp", URL: "https://localhost:3000/taskpane.html"},
		{TargetID: "dlg", URL: "https://localhost:3000/dialog.html"},
		{TargetID: "fn", URL: "https://localhost:3000/functions.html"},
		{TargetID: "blank", URL: "about:blank"},
	}
	out := ClassifyTargets(targets, nil)
	want := map[string]SurfaceType{
		"tp":    SurfaceTaskpane,
		"dlg":   SurfaceDialog,
		"fn":    SurfaceCFRuntime,
		"blank": "",
	}
	for _, c := range out {
		if c.Surface != want[c.TargetID] {
			t.Errorf("target %s: got %q want %q", c.TargetID, c.Surface, want[c.TargetID])
		}
	}
}

func TestFindSurface(t *testing.T) {
	cs := []ClassifiedTarget{
		{TargetInfo: cdp.TargetInfo{TargetID: "x"}, Surface: SurfaceContent},
		{TargetInfo: cdp.TargetInfo{TargetID: "y"}, Surface: SurfaceTaskpane},
	}
	if c, ok := FindSurface(cs, SurfaceTaskpane); !ok || c.TargetID != "y" {
		t.Errorf("got %+v ok=%v", c, ok)
	}
	if _, ok := FindSurface(cs, SurfaceDialog); ok {
		t.Errorf("expected no match")
	}
}
