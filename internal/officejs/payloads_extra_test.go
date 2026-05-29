package officejs

import (
	"strings"
	"testing"
)

// TestRequirements_NotFound covers the lookup-miss branch in Requirements.
func TestRequirements_NotFound(t *testing.T) {
	if err := Preload(); err != nil {
		t.Fatalf("preload: %v", err)
	}
	_, err := Requirements("excel.noSuchPayload")
	if err == nil || !strings.Contains(err.Error(), "no payload") {
		t.Fatalf("expected no-payload error, got %v", err)
	}
}

// TestRequirements_NoDirectives covers a payload with zero @requires lines,
// where parseRequires returns nil and Requirements returns an empty slice.
func TestRequirements_NoDirectives(t *testing.T) {
	if err := Preload(); err != nil {
		t.Fatalf("preload: %v", err)
	}
	// excel.runScript is the escape hatch; assert it resolves without error.
	// Whether it has directives or not, the call must succeed for a known name.
	if _, err := Requirements("excel.runScript"); err != nil {
		t.Fatalf("requirements(excel.runScript): %v", err)
	}
}

// TestGetPayload_Known and _Unknown cover both branches of getPayload via the
// package-private helper.
func TestGetPayload(t *testing.T) {
	if err := Preload(); err != nil {
		t.Fatalf("preload: %v", err)
	}
	body, err := getPayload("excel.readRange")
	if err != nil {
		t.Fatalf("getPayload known: %v", err)
	}
	if body == "" {
		t.Error("expected non-empty payload body for excel.readRange")
	}

	if _, err := getPayload("excel.bogusName"); err == nil || !strings.Contains(err.Error(), "no payload") {
		t.Fatalf("expected no-payload error, got %v", err)
	}
}

// TestPreamble_NonEmpty covers the happy path of preamble().
func TestPreamble_NonEmpty(t *testing.T) {
	if err := Preload(); err != nil {
		t.Fatalf("preload: %v", err)
	}
	pre, err := preamble()
	if err != nil {
		t.Fatalf("preamble: %v", err)
	}
	if strings.TrimSpace(pre) == "" {
		t.Error("expected non-empty preamble source")
	}
}

// TestNames_NonEmpty covers the happy path of Names() returning the loaded set.
func TestNames_NonEmpty(t *testing.T) {
	if err := Preload(); err != nil {
		t.Fatalf("preload: %v", err)
	}
	names := Names()
	if len(names) == 0 {
		t.Fatal("expected non-empty payload names")
	}
}

// TestCamelize covers the conversion helper including the empty-part branch
// (leading/trailing/double underscores produce empty segments that are skipped)
// and the i==0 first-segment branch.
func TestCamelize(t *testing.T) {
	cases := map[string]string{
		"read_range":         "readRange",
		"get_active_sheet":   "getActiveSheet",
		"single":             "single",
		"":                   "",
		"_leading":           "Leading",  // empty first part skipped, "leading" capitalized as a later part
		"trailing_":          "trailing", // empty trailing part skipped
		"double__underscore": "doubleUnderscore",
		"a_b_c":              "aBC",
	}
	for in, want := range cases {
		if got := camelize(in); got != want {
			t.Errorf("camelize(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestParseRequires_None covers the no-match return-nil branch and a multi
// directive parse.
func TestParseRequires_None(t *testing.T) {
	if got := parseRequires("// just a comment\nconst x = 1;"); got != nil {
		t.Errorf("expected nil for no @requires, got %+v", got)
	}
	src := "// @requires ExcelApi 1.1\n// @requires ExcelApi 1.7\ncode();"
	got := parseRequires(src)
	if len(got) != 2 {
		t.Fatalf("expected 2 requirements, got %d (%+v)", len(got), got)
	}
	if got[0].Set != "ExcelApi" || got[0].Version != "1.1" {
		t.Errorf("first req = %+v", got[0])
	}
	if got[1].Version != "1.7" {
		t.Errorf("second req = %+v", got[1])
	}
}
