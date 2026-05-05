package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/doccache"
)

func TestClassifyOfficeJSErr_LeavesNonOfficeJSAlone(t *testing.T) {
	errEnv := &EnvelopeError{
		Code:     "fake",
		Message:  "x",
		Category: CategoryProtocol,
	}
	classifyOfficeJSErr(context.Background(), nil, "excel.tabulateRegion", []byte(`{}`), errEnv)
	if errEnv.Details != nil {
		t.Fatalf("non-office_js error should not be enriched, got Details=%v", errEnv.Details)
	}
}

func TestClassifyOfficeJSErr_Excel_ItemNotFound_DocCache(t *testing.T) {
	store := openTestStore(t)
	mustPut(t, store, doccache.Entry{
		Host:        "excel",
		FilePath:    "Book1.xlsx",
		Fingerprint: "fp1",
		Data:        json.RawMessage(`{"worksheets":[{"name":"Inputs"},{"name":"Outputs"},{"name":"Summary"}]}`),
	})
	env := &RunEnv{DocCache: store}

	errEnv := &EnvelopeError{
		Code:     "ItemNotFound",
		Message:  "Sheet not found",
		Category: CategoryOfficeJS,
	}
	classifyOfficeJSErr(context.Background(), env, "excel.tabulateRegion",
		[]byte(`{"address":"Inputz!A1:B2"}`), errEnv)

	if got := errEnv.Details["failing_address"]; got != "Inputz!A1:B2" {
		t.Errorf("failing_address=%v", got)
	}
	if got := errEnv.Details["available_sheets_source"]; got != "doccache" {
		t.Errorf("available_sheets_source=%v want doccache", got)
	}
	sheets, _ := errEnv.Details["available_sheets"].([]string)
	if len(sheets) != 3 {
		t.Fatalf("available_sheets len=%d want 3 (%v)", len(sheets), sheets)
	}
	suggestions, _ := errEnv.Details["nearest_name_suggestions"].([]string)
	if len(suggestions) == 0 || suggestions[0] != "Inputs" {
		t.Errorf("nearest_name_suggestions=%v want first=Inputs", suggestions)
	}
	if errEnv.RecoveryHint == "" {
		t.Error("RecoveryHint should be populated for ItemNotFound")
	}
}

func TestClassifyOfficeJSErr_Excel_InvalidArgument_AddressBounds(t *testing.T) {
	env := &RunEnv{}
	errEnv := &EnvelopeError{
		Code:     "InvalidArgument",
		Message:  "bad range",
		Category: CategoryOfficeJS,
	}
	classifyOfficeJSErr(context.Background(), env, "excel.tabulateRegion",
		[]byte(`{"address":"Sheet1!A1:ZZZZ5"}`), errEnv)

	if got := errEnv.Details["failing_address"]; got != "Sheet1!A1:ZZZZ5" {
		t.Errorf("failing_address=%v", got)
	}
	if errEnv.Details["column_out_of_bounds"] != "ZZZZ" {
		t.Errorf("column_out_of_bounds=%v want ZZZZ", errEnv.Details["column_out_of_bounds"])
	}
	parsed, _ := errEnv.Details["parsed_address"].(map[string]any)
	if parsed["start_column"] != "A" {
		t.Errorf("parsed_address.start_column=%v", parsed["start_column"])
	}
	if errEnv.RecoveryHint == "" {
		t.Error("RecoveryHint should be populated for InvalidArgument with parseable address")
	}
}

func TestClassifyOfficeJSErr_PowerPoint_DocCache(t *testing.T) {
	store := openTestStore(t)
	mustPut(t, store, doccache.Entry{
		Host:        "powerpoint",
		FilePath:    "Deck.pptx",
		Fingerprint: "fp1",
		Data:        json.RawMessage(`{"slideCount":7,"shapeCount":21}`),
	})
	env := &RunEnv{DocCache: store}

	errEnv := &EnvelopeError{
		Code:     "InvalidArgument",
		Message:  "slide index out of range",
		Category: CategoryOfficeJS,
	}
	classifyOfficeJSErr(context.Background(), env, "powerpoint.rebuildSlideFromOutline",
		[]byte(`{"slideIndex":99}`), errEnv)

	if got := errEnv.Details["slide_count"]; got != 7 {
		t.Errorf("slide_count=%v want 7", got)
	}
	if errEnv.RecoveryHint == "" {
		t.Error("RecoveryHint should be populated when slide count is known")
	}
}

func TestClassifyOfficeJSErr_Outlook_ComposeReadMismatch(t *testing.T) {
	store := openTestStore(t)
	mustPut(t, store, doccache.Entry{
		Host:        "outlook",
		FilePath:    "user@example.com",
		Fingerprint: "fp1",
		Data:        json.RawMessage(`{"hostMode":"messageRead"}`),
	})
	env := &RunEnv{DocCache: store}

	errEnv := &EnvelopeError{
		Code:     "InvalidOperation",
		Message:  "Property is only available in compose mode.",
		Category: CategoryOfficeJS,
	}
	classifyOfficeJSErr(context.Background(), env, "outlook.draftReply",
		[]byte(`{"body":"hi"}`), errEnv)

	if got := errEnv.Details["item_mode"]; got != "messageRead" {
		t.Errorf("item_mode=%v want messageRead", got)
	}
}

func TestAnalyzeAddress(t *testing.T) {
	cases := []struct {
		addr      string
		wantOOB   bool
		wantStart string
	}{
		{"A1:B2", false, "A"},
		{"Sheet1!A1:B2", false, "A"},
		{"Sheet1!XFE1", true, "XFE"},
		{"Sheet1!A1:ZZZZ5", true, "A"},
		{"NotARange", false, ""},
	}
	for _, tc := range cases {
		got := analyzeAddress(tc.addr)
		if tc.wantStart == "" {
			if got != nil {
				t.Errorf("analyzeAddress(%q)=%v want nil", tc.addr, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("analyzeAddress(%q)=nil want non-nil", tc.addr)
			continue
		}
		parsed, _ := got["parsed_address"].(map[string]any)
		if parsed["start_column"] != tc.wantStart {
			t.Errorf("addr=%q start_column=%v want %v", tc.addr, parsed["start_column"], tc.wantStart)
		}
		_, hasOOB := got["column_out_of_bounds"]
		if hasOOB != tc.wantOOB {
			t.Errorf("addr=%q column_out_of_bounds present=%v want %v", tc.addr, hasOOB, tc.wantOOB)
		}
	}
}

func TestNearestNames(t *testing.T) {
	names := []string{"Sheet1", "Sheet2", "Inputs", "Outputs", "Summary"}
	got := nearestNames("inputz", names, 2)
	if len(got) == 0 || got[0] != "Inputs" {
		t.Errorf("nearestNames(inputz)=%v want first=Inputs", got)
	}
	if len(nearestNames("", names, 3)) != 0 {
		t.Error("empty query should return nil")
	}
	if len(nearestNames("zzzzzzzzzzzzz", names, 3)) != 0 {
		t.Error("very-far query should return nil after distance cap")
	}
}

func TestSheetFromAddress(t *testing.T) {
	cases := map[string]string{
		"Sheet1!A1:B2":   "Sheet1",
		"'Has Space'!A1": "Has Space",
		"'O''Brien'!A1":  "O'Brien",
		"A1":             "",
		"":               "",
	}
	for in, want := range cases {
		if got := sheetFromAddress(in); got != want {
			t.Errorf("sheetFromAddress(%q)=%q want %q", in, got, want)
		}
	}
}

func TestHostFromTool(t *testing.T) {
	cases := map[string]string{
		"excel.tabulateRegion":    "excel",
		"powerpoint.rebuildSlide": "powerpoint",
		"office.embed":            "office",
		"":                        "",
		"noDot":                   "",
	}
	for in, want := range cases {
		if got := hostFromTool(in); got != want {
			t.Errorf("hostFromTool(%q)=%q want %q", in, got, want)
		}
	}
}

func openTestStore(t *testing.T) *doccache.Store {
	t.Helper()
	dir := t.TempDir()
	return doccache.Open(filepath.Join(dir, "doccache.json"), false)
}

func mustPut(t *testing.T, s *doccache.Store, e doccache.Entry) {
	t.Helper()
	if err := s.Put(e); err != nil {
		t.Fatalf("put: %v", err)
	}
}
