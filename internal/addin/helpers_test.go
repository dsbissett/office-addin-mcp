package addin

import (
	"reflect"
	"testing"
)

func TestSurfaceForExtensionPoint(t *testing.T) {
	cases := []struct {
		name    string
		xsiType string
		want    SurfaceType
	}{
		{"customFunctions", "Office.CustomFunctions", SurfaceCFRuntime},
		{"taskpane", "TaskPaneApp", SurfaceTaskpane},
		{"contentArea", "ContentArea", SurfaceContent},
		{"emptyDefaultsToCommands", "", SurfaceCommands},
		{"unknownDefaultsToCommands", "PrimaryCommandSurface", SurfaceCommands},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := surfaceForExtensionPoint(tc.xsiType, "https://example.com/x.html")
			if got.Type != tc.want {
				t.Errorf("type = %q, want %q", got.Type, tc.want)
			}
			if got.URL != "https://example.com/x.html" {
				t.Errorf("URL = %q", got.URL)
			}
			if got.Pattern != "example.com/x.html" {
				t.Errorf("Pattern = %q", got.Pattern)
			}
		})
	}
}

func TestContainsFold(t *testing.T) {
	if !containsFold("Office.CustomFunctions", "customfunctions") {
		t.Errorf("expected case-insensitive match")
	}
	if containsFold("TaskPane", "dialog") {
		t.Errorf("unexpected match")
	}
	if !containsFold("ABC", "") {
		t.Errorf("empty substring should always match")
	}
}

func TestJSONScopeToHost(t *testing.T) {
	cases := map[string]string{
		"workbook":     "Workbook",
		"WORKBOOK":     "Workbook",
		"document":     "Document",
		"presentation": "Presentation",
		"mailbox":      "Mailbox",
		"something":    "something", // unknown passes through unchanged
	}
	for in, want := range cases {
		if got := jsonScopeToHost(in); got != want {
			t.Errorf("jsonScopeToHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHasCustomFunctionsAction(t *testing.T) {
	type action = struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if !hasCustomFunctionsAction([]action{{Type: "customFunction"}}) {
		t.Errorf("customFunction not detected")
	}
	if !hasCustomFunctionsAction([]action{{Type: "ExecuteFunction"}}) {
		t.Errorf("executeFunction (case-insensitive) not detected")
	}
	if hasCustomFunctionsAction([]action{{Type: "openPage"}}) {
		t.Errorf("openPage should not be a custom function")
	}
	if hasCustomFunctionsAction(nil) {
		t.Errorf("nil actions should be false")
	}
}

func TestResolveResID(t *testing.T) {
	urls := map[string]string{
		"present": "https://example.com/p.html",
		"empty":   "",
	}
	if v, ok := resolveResID(urls, "present"); !ok || v != "https://example.com/p.html" {
		t.Errorf("present: v=%q ok=%v", v, ok)
	}
	if _, ok := resolveResID(urls, ""); ok {
		t.Errorf("empty resID should be false")
	}
	if _, ok := resolveResID(urls, "missing"); ok {
		t.Errorf("missing key should be false")
	}
	if _, ok := resolveResID(urls, "empty"); ok {
		t.Errorf("empty value should be false")
	}
}

func TestUrlPattern_EdgeCases(t *testing.T) {
	cases := map[string]string{
		"":                          "",                  // empty in -> empty out
		"https://example.com/":      "example.com",       // root path -> host only
		"https://example.com":       "example.com",       // no path -> host only
		"https://h.io/a/b.html":     "h.io/a/b.html",     // host+path
		"file:///c/foo/bar.html":    "bar.html",          // no host -> basename
		"relative/path/index.html":  "index.html",        // no scheme/host -> basename
		"webview://abc/page.html":   "abc/page.html",     // custom scheme parses host=abc
		"https://localhost:3000/tp": "localhost:3000/tp", // host:port + path
	}
	for in, want := range cases {
		if got := urlPattern(in); got != want {
			t.Errorf("urlPattern(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUrlPattern_ParseError(t *testing.T) {
	// A control byte makes url.Parse fail; fall back to basename.
	in := "ht\x7ftp://bad\nhost/seg/last.html"
	got := urlPattern(in)
	if got != "last.html" {
		t.Errorf("urlPattern(parse-error) = %q, want last.html", got)
	}
}

func TestAppendStringUnique(t *testing.T) {
	out := appendStringUnique(nil, "a")
	out = appendStringUnique(out, "b")
	out = appendStringUnique(out, "a") // duplicate ignored
	out = appendStringUnique(out, "")  // empty ignored
	if !reflect.DeepEqual(out, []string{"a", "b"}) {
		t.Errorf("got %v, want [a b]", out)
	}
}

func TestAppendRequirementUnique(t *testing.T) {
	r1 := RequirementSet{Name: "ExcelApi", MinVersion: "1.1"}
	r2 := RequirementSet{Name: "WordApi", MinVersion: "1.3"}
	out := appendRequirementUnique(nil, r1)
	out = appendRequirementUnique(out, r2)
	out = appendRequirementUnique(out, RequirementSet{Name: "ExcelApi", MinVersion: "9.9"}) // same Name -> ignored
	if len(out) != 2 {
		t.Errorf("len = %d, want 2 (%v)", len(out), out)
	}
}

func TestAppendSurfaceUnique(t *testing.T) {
	s1 := Surface{Type: SurfaceTaskpane, URL: "https://x/y", Pattern: "x/y"}
	out := appendSurfaceUnique(nil, s1)
	out = appendSurfaceUnique(out, s1)                                                 // exact dup ignored
	out = appendSurfaceUnique(out, Surface{Type: SurfaceTaskpane, URL: ""})            // empty URL ignored
	out = appendSurfaceUnique(out, Surface{Type: SurfaceContent, URL: "https://x/y"})  // same URL diff type kept
	out = appendSurfaceUnique(out, Surface{Type: SurfaceTaskpane, URL: "https://x/z"}) // new URL kept
	if len(out) != 3 {
		t.Errorf("len = %d, want 3 (%v)", len(out), out)
	}
}

func TestMergeRequirementSets(t *testing.T) {
	base := []RequirementSet{
		{Name: "ExcelApi", MinVersion: "1.1"},
		{Name: "ExcelApi", MinVersion: "1.7"},
	}
	extras := []RequirementSet{
		{Name: "ExcelApi", MinVersion: "1.7"},  // exact dup of base -> skipped
		{Name: "ExcelApi", MinVersion: "1.99"}, // new minVersion -> added
		{Name: "WordApi", MinVersion: "1.1"},   // new name -> added
	}
	out := MergeRequirementSets(base, extras)
	keys := requirementKeys(out)
	for _, want := range []string{"ExcelApi@1.1", "ExcelApi@1.7", "ExcelApi@1.99", "WordApi@1.1"} {
		if !keys[want] {
			t.Errorf("missing %q in %v", want, out)
		}
	}
	if len(out) != 4 {
		t.Errorf("len = %d, want 4 (%v)", len(out), out)
	}
	// base must not be mutated.
	if len(base) != 2 {
		t.Errorf("base mutated: %v", base)
	}
}

func TestMergeRequirementSets_EmptyExtras(t *testing.T) {
	base := []RequirementSet{{Name: "ExcelApi", MinVersion: "1.1"}}
	out := MergeRequirementSets(base, nil)
	if !reflect.DeepEqual(out, base) {
		t.Errorf("got %v, want %v", out, base)
	}
}

func TestStandardRequirementSets_NonEmpty(t *testing.T) {
	if len(StandardRequirementSets) == 0 {
		t.Fatalf("StandardRequirementSets is empty")
	}
	// Merging the standard list with itself must be a no-op (full dedup).
	merged := MergeRequirementSets(StandardRequirementSets, StandardRequirementSets)
	if len(merged) != len(StandardRequirementSets) {
		t.Errorf("self-merge changed length: %d vs %d", len(merged), len(StandardRequirementSets))
	}
}
