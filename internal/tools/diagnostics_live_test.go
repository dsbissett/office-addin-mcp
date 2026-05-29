package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/doccache"
)

// diagEnv returns a RunEnv whose Attach hands the diagnostics enrichers a real
// *cdp.Connection backed by an in-process CDP server driven by resp. No
// DocCache is wired, so lookups fall through to the live Office.js payload.
func diagEnv(t *testing.T, resp cdptest.Responder) *RunEnv {
	t.Helper()
	srv := cdptest.NewServer(t, resp)
	return &RunEnv{
		Diag: &Diagnostics{},
		Attach: func(context.Context, TargetSelector) (*AttachedTarget, error) {
			return &AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
		},
	}
}

func TestClassifyOfficeJSErr_Excel_ItemNotFound_LiveLookup(t *testing.T) {
	env := diagEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"worksheets": []map[string]any{
					{"name": "Inputs"}, {"name": "Outputs"}, {"name": ""},
				},
			}), nil
		}
		return map[string]any{}, nil
	})
	errEnv := &EnvelopeError{Code: "ItemNotFound", Message: "no sheet", Category: CategoryOfficeJS}
	classifyOfficeJSErr(context.Background(), env, "excel.tabulateRegion",
		json.RawMessage(`{"address":"Inputz!A1:B2"}`), errEnv)

	sheets, _ := errEnv.Details["available_sheets"].([]string)
	if len(sheets) != 2 { // empty name filtered out
		t.Fatalf("available_sheets=%v want 2 (empty filtered)", sheets)
	}
	if errEnv.Details["available_sheets_source"] != "live" {
		t.Errorf("source=%v want live", errEnv.Details["available_sheets_source"])
	}
	sugg, _ := errEnv.Details["nearest_name_suggestions"].([]string)
	if len(sugg) == 0 || sugg[0] != "Inputs" {
		t.Errorf("nearest=%v want first Inputs", sugg)
	}
}

func TestClassifyOfficeJSErr_Excel_ItemNotFound_NoSheetsHint(t *testing.T) {
	// Live lookup returns no worksheets — enrichExcel still sets the fallback
	// recovery hint and leaves available_sheets unset.
	env := diagEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"worksheets": []map[string]any{}}), nil
		}
		return map[string]any{}, nil
	})
	errEnv := &EnvelopeError{Code: "ItemNotFound", Message: "no sheet", Category: CategoryOfficeJS}
	classifyOfficeJSErr(context.Background(), env, "excel.tabulateRegion",
		json.RawMessage(`{"address":"A1"}`), errEnv)
	if _, ok := errEnv.Details["available_sheets"]; ok {
		t.Error("available_sheets should be unset when none found")
	}
	if errEnv.RecoveryHint == "" {
		t.Error("fallback RecoveryHint should be set when sheets unknown")
	}
}

func TestClassifyOfficeJSErr_Excel_ItemNotFound_AttachFails(t *testing.T) {
	// Attach fails: lookupExcelSheets returns nothing, fallback hint set.
	env := &RunEnv{
		Diag: &Diagnostics{},
		Attach: func(context.Context, TargetSelector) (*AttachedTarget, error) {
			return nil, errors.New("no target")
		},
	}
	errEnv := &EnvelopeError{Code: "ItemNotFound", Message: "x", Category: CategoryOfficeJS}
	classifyOfficeJSErr(context.Background(), env, "excel.tabulateRegion",
		json.RawMessage(`{"address":"Foo!A1"}`), errEnv)
	if _, ok := errEnv.Details["available_sheets"]; ok {
		t.Error("available_sheets should be unset on attach failure")
	}
	if errEnv.RecoveryHint == "" {
		t.Error("fallback hint should still be set")
	}
}

func TestClassifyOfficeJSErr_Excel_OtherCode_NoEnrichment(t *testing.T) {
	// enrichExcel returns early for codes other than ItemNotFound/InvalidArgument.
	env := &RunEnv{}
	errEnv := &EnvelopeError{Code: "GeneralException", Message: "x", Category: CategoryOfficeJS}
	classifyOfficeJSErr(context.Background(), env, "excel.tabulateRegion",
		json.RawMessage(`{"address":"Sheet1!A1"}`), errEnv)
	// failing_address still recorded, but no sheet/parse enrichment.
	if errEnv.Details["failing_address"] != "Sheet1!A1" {
		t.Errorf("failing_address=%v", errEnv.Details["failing_address"])
	}
	if _, ok := errEnv.Details["parsed_address"]; ok {
		t.Error("parsed_address should be absent for non-InvalidArgument code")
	}
}

func TestClassifyOfficeJSErr_Excel_InvalidArgument_NoAddress(t *testing.T) {
	// InvalidArgument without an address param: nothing to parse, no hint set
	// by the address branch.
	env := &RunEnv{}
	errEnv := &EnvelopeError{Code: "InvalidArgument", Message: "x", Category: CategoryOfficeJS}
	classifyOfficeJSErr(context.Background(), env, "excel.tabulateRegion",
		json.RawMessage(`{}`), errEnv)
	if _, ok := errEnv.Details["parsed_address"]; ok {
		t.Error("parsed_address should be absent without address")
	}
}

func TestClassifyOfficeJSErr_PowerPoint_LiveLookup(t *testing.T) {
	env := diagEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"slideCount": 4}), nil
		}
		return map[string]any{}, nil
	})
	errEnv := &EnvelopeError{Code: "InvalidArgument", Message: "out of range", Category: CategoryOfficeJS}
	classifyOfficeJSErr(context.Background(), env, "powerpoint.rebuildSlideFromOutline",
		json.RawMessage(`{"slideIndex":99}`), errEnv)
	if errEnv.Details["slide_count"] != 4 {
		t.Errorf("slide_count=%v want 4", errEnv.Details["slide_count"])
	}
	if errEnv.RecoveryHint == "" {
		t.Error("hint should be set when slide count known")
	}
}

func TestClassifyOfficeJSErr_PowerPoint_LookupMiss(t *testing.T) {
	// Live lookup returns 0 slides → lookupPowerPointSlideCount reports !ok and
	// enrichPowerPoint returns without setting slide_count.
	env := diagEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"slideCount": 0}), nil
		}
		return map[string]any{}, nil
	})
	errEnv := &EnvelopeError{Code: "ItemNotFound", Message: "x", Category: CategoryOfficeJS}
	classifyOfficeJSErr(context.Background(), env, "powerpoint.rebuildSlideFromOutline",
		json.RawMessage(`{}`), errEnv)
	if _, ok := errEnv.Details["slide_count"]; ok {
		t.Error("slide_count should be unset when lookup misses")
	}
}

func TestClassifyOfficeJSErr_PowerPoint_OtherCode(t *testing.T) {
	// enrichPowerPoint early-returns for unrelated codes; never calls the lookup.
	env := diagEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		t.Error("lookup should not be invoked for unrelated code")
		return map[string]any{}, nil
	})
	errEnv := &EnvelopeError{Code: "GeneralException", Message: "x", Category: CategoryOfficeJS}
	classifyOfficeJSErr(context.Background(), env, "powerpoint.rebuildSlideFromOutline",
		json.RawMessage(`{}`), errEnv)
	if _, ok := errEnv.Details["slide_count"]; ok {
		t.Error("slide_count should be unset")
	}
}

func TestClassifyOfficeJSErr_Outlook_LiveLookup(t *testing.T) {
	env := diagEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"hostMode": "messageCompose"}), nil
		}
		return map[string]any{}, nil
	})
	errEnv := &EnvelopeError{
		Code:     "InvalidOperation",
		Message:  "only available in read mode",
		Category: CategoryOfficeJS,
	}
	classifyOfficeJSErr(context.Background(), env, "outlook.draftReply",
		json.RawMessage(`{}`), errEnv)
	if errEnv.Details["item_mode"] != "messageCompose" {
		t.Errorf("item_mode=%v want messageCompose", errEnv.Details["item_mode"])
	}
	if errEnv.RecoveryHint == "" {
		t.Error("hint should be set when item mode known")
	}
}

func TestClassifyOfficeJSErr_Outlook_NoHit(t *testing.T) {
	// Message/code don't match any compose/read trigger → enrichOutlook returns
	// without consulting the lookup.
	env := diagEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		t.Error("lookup should not run when no compose/read hit")
		return map[string]any{}, nil
	})
	errEnv := &EnvelopeError{
		Code:     "GeneralException",
		Message:  "something unrelated",
		Category: CategoryOfficeJS,
	}
	classifyOfficeJSErr(context.Background(), env, "outlook.draftReply",
		json.RawMessage(`{}`), errEnv)
	if _, ok := errEnv.Details["item_mode"]; ok {
		t.Error("item_mode should be unset when no trigger matched")
	}
}

func TestClassifyOfficeJSErr_Outlook_HitButLookupMiss(t *testing.T) {
	// Trigger matches (ItemNotFound) but live lookup returns empty hostMode.
	env := diagEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"hostMode": ""}), nil
		}
		return map[string]any{}, nil
	})
	errEnv := &EnvelopeError{Code: "ItemNotFound", Message: "x", Category: CategoryOfficeJS}
	classifyOfficeJSErr(context.Background(), env, "outlook.draftReply",
		json.RawMessage(`{}`), errEnv)
	if _, ok := errEnv.Details["item_mode"]; ok {
		t.Error("item_mode should be unset when lookup misses")
	}
}

func TestClassifyOfficeJSErr_UnknownHost(t *testing.T) {
	// hostFromTool yields a host with no enricher (e.g. word/onenote/office) —
	// switch falls through, only failing_address recorded.
	env := &RunEnv{}
	errEnv := &EnvelopeError{Code: "GeneralException", Message: "x", Category: CategoryOfficeJS}
	classifyOfficeJSErr(context.Background(), env, "word.applyEdits",
		json.RawMessage(`{"address":"unused"}`), errEnv)
	if errEnv.Details["failing_address"] != "unused" {
		t.Errorf("failing_address=%v", errEnv.Details["failing_address"])
	}
}

func TestRunDiagnosticsPayload_NoAttach(t *testing.T) {
	_, err := runDiagnosticsPayload(context.Background(), nil,
		json.RawMessage(`{}`), "excel.listWorksheets", nil)
	if err == nil {
		t.Fatal("expected error when env is nil")
	}

	envNoAttach := &RunEnv{}
	_, err = runDiagnosticsPayload(context.Background(), envNoAttach,
		json.RawMessage(`{}`), "excel.listWorksheets", nil)
	if err == nil {
		t.Fatal("expected error when env.Attach is nil")
	}
}

func TestRunDiagnosticsPayload_AttachError(t *testing.T) {
	env := &RunEnv{
		Attach: func(context.Context, TargetSelector) (*AttachedTarget, error) {
			return nil, errors.New("attach boom")
		},
	}
	_, err := runDiagnosticsPayload(context.Background(), env,
		json.RawMessage(`{}`), "excel.listWorksheets", nil)
	if err == nil {
		t.Fatal("expected attach error to propagate")
	}
}

func TestLookupExcelSheets_LiveUnmarshalError(t *testing.T) {
	// Payload result is not the expected worksheets shape → json.Unmarshal of
	// the typed struct still succeeds (extra fields ignored) but yields no
	// names, so lookup returns "" source.
	env := diagEnv(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{"unexpected": true}), nil
		}
		return map[string]any{}, nil
	})
	names, source := lookupExcelSheets(context.Background(), env, json.RawMessage(`{}`))
	if len(names) != 0 || source != "" {
		t.Errorf("names=%v source=%q want empty", names, source)
	}
}

func TestLookupExcelSheets_DocCacheEmptyFallsToLive(t *testing.T) {
	// A doccache entry exists but its Data yields no worksheet names, so the
	// lookup falls through to the live Office.js payload.
	srv := cdptest.NewServer(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(map[string]any{
				"worksheets": []map[string]any{{"name": "Live1"}},
			}), nil
		}
		return map[string]any{}, nil
	})
	store := openTestStore(t)
	mustPut(t, store, doccache.Entry{
		Host:        "excel",
		FilePath:    "Empty.xlsx",
		Fingerprint: "fp",
		Data:        json.RawMessage(`{"worksheets":[]}`),
	})
	env := &RunEnv{
		Diag:     &Diagnostics{},
		DocCache: store,
		Attach: func(context.Context, TargetSelector) (*AttachedTarget, error) {
			return &AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
		},
	}
	names, source := lookupExcelSheets(context.Background(), env, json.RawMessage(`{}`))
	if source != "live" || len(names) != 1 || names[0] != "Live1" {
		t.Errorf("names=%v source=%q want live [Live1]", names, source)
	}
}

func TestSheetsFromCacheData_Edges(t *testing.T) {
	if got := sheetsFromCacheData(nil); got != nil {
		t.Errorf("nil data => %v want nil", got)
	}
	if got := sheetsFromCacheData(json.RawMessage(`not json`)); got != nil {
		t.Errorf("bad json => %v want nil", got)
	}
	got := sheetsFromCacheData(json.RawMessage(`{"worksheets":[{"name":"A"},{"name":""},{"name":"B"}]}`))
	if len(got) != 2 || got[0] != "A" || got[1] != "B" {
		t.Errorf("got %v want [A B]", got)
	}
}

func TestSlideCountFromCacheData_Edges(t *testing.T) {
	if _, ok := slideCountFromCacheData(nil); ok {
		t.Error("nil data should report !ok")
	}
	if _, ok := slideCountFromCacheData(json.RawMessage(`bad`)); ok {
		t.Error("bad json should report !ok")
	}
	if _, ok := slideCountFromCacheData(json.RawMessage(`{"slideCount":0}`)); ok {
		t.Error("zero count should report !ok")
	}
	n, ok := slideCountFromCacheData(json.RawMessage(`{"slideCount":5}`))
	if !ok || n != 5 {
		t.Errorf("got (%d,%v) want (5,true)", n, ok)
	}
}

func TestItemModeFromCacheData_Edges(t *testing.T) {
	if _, ok := itemModeFromCacheData(nil); ok {
		t.Error("nil data should report !ok")
	}
	if _, ok := itemModeFromCacheData(json.RawMessage(`bad`)); ok {
		t.Error("bad json should report !ok")
	}
	if _, ok := itemModeFromCacheData(json.RawMessage(`{"hostMode":""}`)); ok {
		t.Error("empty hostMode should report !ok")
	}
	m, ok := itemModeFromCacheData(json.RawMessage(`{"hostMode":"messageRead"}`))
	if !ok || m != "messageRead" {
		t.Errorf("got (%q,%v) want (messageRead,true)", m, ok)
	}
}

func TestExtractParamString_Edges(t *testing.T) {
	if got := extractParamString(nil, "k"); got != "" {
		t.Errorf("nil params => %q", got)
	}
	if got := extractParamString(json.RawMessage(`not json`), "k"); got != "" {
		t.Errorf("bad json => %q", got)
	}
	if got := extractParamString(json.RawMessage(`{"other":"v"}`), "k"); got != "" {
		t.Errorf("missing key => %q", got)
	}
	if got := extractParamString(json.RawMessage(`{"k":123}`), "k"); got != "" {
		t.Errorf("non-string value => %q want empty", got)
	}
	if got := extractParamString(json.RawMessage(`{"k":"hello"}`), "k"); got != "hello" {
		t.Errorf("got %q want hello", got)
	}
}

func TestAnalyzeAddress_MoreEdges(t *testing.T) {
	// Absolute refs ($) stripped, single cell (no range part), quoted sheet.
	got := analyzeAddress("'My Sheet'!$A$1")
	if got == nil {
		t.Fatal("expected non-nil for single absolute cell")
	}
	parsed, _ := got["parsed_address"].(map[string]any)
	if parsed["start_column"] != "A" || parsed["start_row"] != 1 {
		t.Errorf("parsed=%v", parsed)
	}
	if _, ok := parsed["end_column"]; ok {
		t.Error("single cell should have no end_column")
	}

	// Row out of bounds on the start cell.
	oob := analyzeAddress("A2000000")
	if oob == nil || oob["row_out_of_bounds"] == nil {
		t.Errorf("expected row_out_of_bounds, got %v", oob)
	}

	// End row out of bounds in a range.
	endOOB := analyzeAddress("A1:B2000000")
	if endOOB == nil || endOOB["row_out_of_bounds"] == nil {
		t.Errorf("expected end row_out_of_bounds, got %v", endOOB)
	}

	// Unparseable.
	if analyzeAddress("totally invalid !!") != nil {
		t.Error("garbage should return nil")
	}
}

func TestColumnIndex_Edges(t *testing.T) {
	if got := columnIndex("A"); got != 1 {
		t.Errorf("A=%d want 1", got)
	}
	if got := columnIndex("AA"); got != 27 {
		t.Errorf("AA=%d want 27", got)
	}
	if got := columnIndex("a"); got != 1 {
		t.Errorf("lowercase a=%d want 1 (upper-cased)", got)
	}
	// Non A-Z rune short-circuits to 0.
	if got := columnIndex("A1"); got != 0 {
		t.Errorf("A1=%d want 0 (non-letter)", got)
	}
}

func TestNearestNames_MoreEdges(t *testing.T) {
	if got := nearestNames("q", nil, 3); got != nil {
		t.Errorf("empty names => %v", got)
	}
	if got := nearestNames("q", []string{"a"}, 0); got != nil {
		t.Errorf("limit 0 => %v", got)
	}
	// limit larger than candidate count is clamped.
	got := nearestNames("inputs", []string{"Inputs", "Inputz"}, 10)
	if len(got) != 2 {
		t.Errorf("got %v want 2 (limit clamped)", got)
	}
	// Stable ordering for ties: two names equidistant keep input order.
	tie := nearestNames("ab", []string{"ac", "ad"}, 2)
	if len(tie) != 2 || tie[0] != "ac" || tie[1] != "ad" {
		t.Errorf("tie order=%v want [ac ad]", tie)
	}
}

func TestLevenshtein_Edges(t *testing.T) {
	if got := levenshtein("", "abc"); got != 3 {
		t.Errorf("('',abc)=%d want 3", got)
	}
	if got := levenshtein("abc", ""); got != 3 {
		t.Errorf("(abc,'')=%d want 3", got)
	}
	if got := levenshtein("abc", "abc"); got != 0 {
		t.Errorf("equal=%d want 0", got)
	}
	if got := levenshtein("kitten", "sitting"); got != 3 {
		t.Errorf("(kitten,sitting)=%d want 3", got)
	}
	if got := levenshtein("flaw", "lawn"); got != 2 {
		t.Errorf("(flaw,lawn)=%d want 2", got)
	}
}
