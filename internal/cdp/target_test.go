package cdp

import "testing"

func TestFirstPageTargetSkipsInternal(t *testing.T) {
	in := []TargetInfo{
		{TargetID: "a", Type: "page", URL: "devtools://devtools/bundled/inspector.html"},
		{TargetID: "b", Type: "service_worker", URL: "https://app.example/sw.js"},
		{TargetID: "c", Type: "page", URL: "chrome://newtab/"},
		{TargetID: "d", Type: "page", URL: "https://app.example/index.html"},
	}
	got, ok := FirstPageTarget(in)
	if !ok {
		t.Fatal("expected a page target")
	}
	if got.TargetID != "d" {
		t.Errorf("got %q, want d", got.TargetID)
	}
}

func TestFirstPageTargetNone(t *testing.T) {
	in := []TargetInfo{{TargetID: "x", Type: "service_worker"}}
	if _, ok := FirstPageTarget(in); ok {
		t.Fatal("expected no page target")
	}
}
