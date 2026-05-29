package exceltool

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// runFn is the shared signature of every excel.* run* entry point.
type runFn func(context.Context, json.RawMessage, *tools.RunEnv) tools.Result

// okEnv builds a RunEnv whose payload always succeeds with data.
func okEnv(t *testing.T, data any) *tools.RunEnv {
	t.Helper()
	return fakeEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOffice(data), nil
	})
}

// runOK drives fn against an okEnv that returns data and asserts success +
// expected summary.
func runOK(t *testing.T, fn runFn, raw string, data any, wantSummary string) tools.Result {
	t.Helper()
	res := fn(context.Background(), json.RawMessage(raw), okEnv(t, data))
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if wantSummary != "" && res.Summary != wantSummary {
		t.Errorf("summary=%q want %q", res.Summary, wantSummary)
	}
	return res
}

// assertOfficeErr drives fn with an Office.js error responder and asserts the
// office_js classification.
func assertOfficeErr(t *testing.T, fn runFn, raw string) {
	t.Helper()
	env := fakeEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOfficeErr("ItemNotFound", "not found", nil), nil
	})
	res := fn(context.Background(), json.RawMessage(raw), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "ItemNotFound" {
		t.Fatalf("want office_js/ItemNotFound, got %+v", res.Err)
	}
}

// assertAttachFailed drives fn through errEnv and asserts the attach_failed code.
func assertAttachFailed(t *testing.T, fn runFn, raw string) {
	t.Helper()
	res := fn(context.Background(), json.RawMessage(raw), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

// assertParamDecode drives fn with malformed JSON and asserts param_decode.
func assertParamDecode(t *testing.T, fn runFn) {
	t.Helper()
	res := fn(context.Background(), json.RawMessage(`{`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// each run* func exercised through the four standard seams: happy, office
// error, attach failure, and malformed params. The minimal valid params per
// tool are supplied so the happy path reaches runPayloadSum.
func TestAllRunFuncs_StandardSeams(t *testing.T) {
	cases := []struct {
		name string
		fn   runFn
		raw  string
	}{
		{"listWorksheets", runListWorksheets, `{}`},
		{"getActiveWorksheet", runGetActiveWorksheet, `{}`},
		{"worksheetInfo", runWorksheetInfo, `{"sheet":"Sheet1"}`},
		{"activateWorksheet", runActivateWorksheet, `{"name":"Sheet1"}`},
		{"createWorksheet", runCreateWorksheet, `{"name":"New"}`},
		{"deleteWorksheet", runDeleteWorksheet, `{"name":"Old"}`},
		{"readRange", runReadRange, `{"address":"A1:B2"}`},
		{"writeRange", runWriteRange, `{"address":"A1","values":[[1]]}`},
		{"getSelectedRange", runGetSelectedRange, `{}`},
		{"setSelectedRange", runSetSelectedRange, `{"address":"A1"}`},
		{"activeRange", runActiveRange, `{}`},
		{"usedRange", runUsedRange, `{}`},
		{"rangeProperties", runRangeProperties, `{"address":"A1:B2"}`},
		{"rangeFormulas", runRangeFormulas, `{"address":"A1:B2"}`},
		{"rangeSpecialCells", runRangeSpecialCells, `{"cellType":"blanks"}`},
		{"findInRange", runFindInRange, `{"text":"x"}`},
		{"listConditionalFormats", runListConditionalFormats, `{}`},
		{"listDataValidations", runListDataValidations, `{}`},
		{"createTable", runCreateTable, `{"address":"A1:B2"}`},
		{"listTables", runListTables, `{}`},
		{"tableInfo", runTableInfo, `{"name":"T1"}`},
		{"tableRows", runTableRows, `{"name":"T1"}`},
		{"tableFilters", runTableFilters, `{"name":"T1"}`},
		{"workbookInfo", runWorkbookInfo, `{}`},
		{"calculationState", runCalculationState, `{}`},
		{"listNamedItems", runListNamedItems, `{}`},
		{"customXmlParts", runCustomXMLParts, `{}`},
		{"settingsGet", runSettingsGet, `{}`},
		{"listComments", runListComments, `{}`},
		{"listShapes", runListShapes, `{}`},
		{"listCharts", runListCharts, `{}`},
		{"chartInfo", runChartInfo, `{"sheet":"Sheet1","name":"C1"}`},
		{"chartImage", runChartImage, `{"sheet":"Sheet1","name":"C1"}`},
		{"listPivotTables", runListPivotTables, `{}`},
		{"pivotTableInfo", runPivotTableInfo, `{"name":"P1"}`},
		{"pivotTableValues", runPivotTableValues, `{"name":"P1"}`},
		{"runScript", runRunScript, `{"script":"return 1;"}`},
		{"tabulateRegion", runTabulateRegion, `{"address":"A1:B2"}`},
		{"applyDiff", runApplyDiff, `{"patches":[{"address":"A1"}]}`},
		{"summarizeWorkbook", runSummarizeWorkbook, `{}`},
		{"query", runQuery, `{"address":"A1:B2"}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name+"/happy", func(t *testing.T) {
			res := tc.fn(context.Background(), json.RawMessage(tc.raw), okEnv(t, map[string]any{}))
			if res.Err != nil {
				t.Fatalf("unexpected error: %+v", res.Err)
			}
			if res.Summary == "" {
				t.Errorf("expected non-empty summary")
			}
		})
		t.Run(tc.name+"/officeErr", func(t *testing.T) { assertOfficeErr(t, tc.fn, tc.raw) })
		t.Run(tc.name+"/attachFail", func(t *testing.T) { assertAttachFailed(t, tc.fn, tc.raw) })
		t.Run(tc.name+"/paramDecode", func(t *testing.T) { assertParamDecode(t, tc.fn) })
	}
}

// --- summary-builder branch coverage ---------------------------------------

func TestRunGetActiveWorksheet_SummaryBranches(t *testing.T) {
	runOK(t, runGetActiveWorksheet, `{}`, map[string]any{"name": "Data"}, "Active worksheet: Data.")
	runOK(t, runGetActiveWorksheet, `{}`, map[string]any{}, "Returned active worksheet.")
}

func TestRunWorksheetInfo_SummaryBranches(t *testing.T) {
	runOK(t, runWorksheetInfo, `{"sheet":"S"}`, map[string]any{"name": "S", "usedRangeAddress": "A1:C9"}, "Worksheet S: used range A1:C9.")
	runOK(t, runWorksheetInfo, `{}`, map[string]any{"name": "S"}, "Worksheet S.")
	runOK(t, runWorksheetInfo, `{}`, map[string]any{}, "Returned worksheet info.")
}

func TestRunReadRange_SummaryBranches(t *testing.T) {
	// rows>0 && cols>0 with truncated suffix.
	runOK(t, runReadRange, `{"address":"A1:B2"}`,
		map[string]any{"address": "A1:B2", "rowCount": float64(2), "columnCount": float64(2), "truncated": true},
		"Read 2x2 cells from A1:B2 (truncated).")
	// addr present, no dims -> falls to addr-only branch (uses payload addr).
	runOK(t, runReadRange, `{"address":"Z9"}`,
		map[string]any{"address": "Z9"},
		"Read Z9.")
	// no addr in payload, fallback to param address.
	runOK(t, runReadRange, `{"address":"Q1"}`,
		map[string]any{},
		"Read Q1.")
	// sheet supplied -> exercises the args["sheet"] branch.
	runOK(t, runReadRange, `{"address":"A1","sheet":"Data"}`,
		map[string]any{"address": "Data!A1"},
		"Read Data!A1.")
}

func TestRunReadRange_VerbOnlyWhenNoAddr(t *testing.T) {
	// Empty fallback address + empty payload addr -> "<verb> range." branch.
	// activeRange uses fallbackAddr "" so it reaches the bare-verb branch.
	runOK(t, runActiveRange, `{}`, map[string]any{}, "Read active range range.")
}

func TestRunWriteRange_Branches(t *testing.T) {
	// values present + sheet -> happy, exercises args["sheet"] branch.
	runOK(t, runWriteRange, `{"address":"A1","sheet":"Data","values":[[1,2]]}`,
		map[string]any{"address": "Data!A1", "rowCount": float64(1), "columnCount": float64(2)},
		"Wrote 1x2 cells from Data!A1.")
	// formulas only.
	runOK(t, runWriteRange, `{"address":"A1","formulas":[["=1+1"]]}`,
		map[string]any{"address": "A1"}, "Wrote A1.")
	// numberFormat only.
	runOK(t, runWriteRange, `{"address":"A1","numberFormat":"0.00"}`,
		map[string]any{"address": "A1"}, "Wrote A1.")
	// missing payload -> validation error, no attach.
	res := runWriteRange(context.Background(), json.RawMessage(`{"address":"A1"}`), errEnv())
	if res.Err == nil || res.Err.Code != "missing_payload" {
		t.Fatalf("want missing_payload, got %+v", res.Err)
	}
}

func TestRunGetSelectedRange_SummaryBranches(t *testing.T) {
	runOK(t, runGetSelectedRange, `{}`, map[string]any{"address": "B2:C3"}, "Selection at B2:C3.")
	runOK(t, runGetSelectedRange, `{}`, map[string]any{}, "No active selection.")
}

func TestRunSetSelectedRange_WithSheet(t *testing.T) {
	runOK(t, runSetSelectedRange, `{"address":"A1","sheet":"Data"}`, map[string]any{}, "Selected A1.")
}

func TestRunUsedRange_ValuesOnlyExplicit(t *testing.T) {
	// valuesOnly explicitly false exercises the *p.ValuesOnly branch; sheet set.
	runOK(t, runUsedRange, `{"sheet":"S","valuesOnly":false,"includeFormulas":true}`,
		map[string]any{"address": "A1:C3", "rowCount": float64(3), "columnCount": float64(3)},
		"Read used range 3x3 cells from A1:C3.")
}

func TestRunRangeProperties_WithFlags(t *testing.T) {
	runOK(t, runRangeProperties, `{"address":"A1:B2","sheet":"S","includeFormat":true,"includeStyle":true}`,
		map[string]any{"address": "A1:B2", "rowCount": float64(2), "columnCount": float64(2)},
		"Read range properties 2x2 cells from A1:B2.")
}

func TestRunRangeFormulas_AddrFromParam(t *testing.T) {
	runOK(t, runRangeFormulas, `{"address":"A1:A5"}`, map[string]any{}, "Read formulas A1:A5.")
}

func TestRunRangeSpecialCells_SummaryBranches(t *testing.T) {
	runOK(t, runRangeSpecialCells, `{"cellType":"constants","valueType":"numbers","address":"A1:Z9"}`,
		map[string]any{"cellCount": float64(3), "address": "A1,A5,A9"},
		"Found 3 constants cell(s) at A1,A5,A9.")
	runOK(t, runRangeSpecialCells, `{"cellType":"blanks"}`,
		map[string]any{"cellCount": float64(2)},
		"Found 2 blanks cell(s).")
	runOK(t, runRangeSpecialCells, `{"cellType":"formulas"}`,
		map[string]any{"cellCount": float64(0)},
		"No formulas cells found.")
}

func TestRunFindInRange_SummaryBranches(t *testing.T) {
	runOK(t, runFindInRange, `{"text":"foo","completeMatch":true,"matchCase":true}`,
		map[string]any{"cellCount": float64(2), "address": "A1,B2"},
		`Found 2 match(es) for "foo" at A1,B2.`)
	runOK(t, runFindInRange, `{"text":"bar"}`,
		map[string]any{"cellCount": float64(1)},
		`Found 1 match(es) for "bar".`)
	runOK(t, runFindInRange, `{"text":"baz"}`,
		map[string]any{"cellCount": float64(0)},
		`No matches for "baz".`)
}

func TestRunListConditionalFormats_Count(t *testing.T) {
	runOK(t, runListConditionalFormats, `{"address":"A1:B2"}`,
		map[string]any{"rules": []any{map[string]any{}, map[string]any{}}},
		"Listed 2 conditional format(s).")
}

func TestRunListDataValidations_Count(t *testing.T) {
	runOK(t, runListDataValidations, `{"address":"A1:B2","sheet":"S"}`,
		map[string]any{"validations": []any{map[string]any{}}},
		"Listed 1 data validation(s).")
}

func TestRunCreateTable_SummaryBranches(t *testing.T) {
	// name from payload.
	runOK(t, runCreateTable, `{"address":"A1:B2"}`,
		map[string]any{"name": "Table1"},
		"Created table Table1 at A1:B2.")
	// name from param when payload omits it.
	runOK(t, runCreateTable, `{"address":"A1:B2","name":"MyT","sheet":"S","hasHeaders":false}`,
		map[string]any{},
		"Created table MyT at A1:B2.")
	// no name anywhere.
	runOK(t, runCreateTable, `{"address":"A1:B2"}`,
		map[string]any{},
		"Created table at A1:B2.")
}

func TestRunListTables_Count(t *testing.T) {
	runOK(t, runListTables, `{}`,
		map[string]any{"tables": []any{map[string]any{}, map[string]any{}, map[string]any{}}},
		"Listed 3 table(s).")
}

func TestRunTableInfo_SummaryBranches(t *testing.T) {
	runOK(t, runTableInfo, `{"name":"T1"}`, map[string]any{"address": "A1:C9"}, "Table T1 at A1:C9.")
	runOK(t, runTableInfo, `{"name":"T1"}`, map[string]any{}, "Returned info for table T1.")
}

func TestRunTableRows_SummaryBranches(t *testing.T) {
	runOK(t, runTableRows, `{"name":"T1","includeHeaders":true}`,
		map[string]any{"rowCount": float64(5), "columnCount": float64(3), "truncated": true},
		"Read 5 row(s) x 3 column(s) from T1 (truncated).")
	runOK(t, runTableRows, `{"name":"T1"}`,
		map[string]any{"rowCount": float64(0)},
		"Read rows from table T1.")
}

func TestRunTableFilters_Count(t *testing.T) {
	runOK(t, runTableFilters, `{"name":"T1"}`,
		map[string]any{"columns": []any{map[string]any{}, map[string]any{}}},
		"Returned filters for table T1 (2 column(s)).")
}

func TestRunWorkbookInfo_SummaryBranches(t *testing.T) {
	runOK(t, runWorkbookInfo, `{}`, map[string]any{"name": "Book1.xlsx"}, "Returned workbook info for Book1.xlsx.")
	runOK(t, runWorkbookInfo, `{}`, map[string]any{}, "Returned workbook info.")
}

func TestRunCalculationState_SummaryBranches(t *testing.T) {
	runOK(t, runCalculationState, `{}`,
		map[string]any{"calculationMode": "Automatic", "calculationState": "Done"},
		"Calculation mode=Automatic, state=Done.")
	runOK(t, runCalculationState, `{}`,
		map[string]any{"calculationMode": "Manual"},
		"Calculation mode=Manual.")
	runOK(t, runCalculationState, `{}`, map[string]any{}, "Returned calculation state.")
}

func TestRunListNamedItems_Count(t *testing.T) {
	runOK(t, runListNamedItems, `{}`,
		map[string]any{"items": []any{map[string]any{}}},
		"Listed 1 named item(s).")
}

func TestRunCustomXMLParts_Count(t *testing.T) {
	runOK(t, runCustomXMLParts, `{}`,
		map[string]any{"parts": []any{}},
		"Listed 0 custom XML part(s).")
}

func TestRunSettingsGet_SummaryBranches(t *testing.T) {
	// single key.
	runOK(t, runSettingsGet, `{"key":"theme"}`, map[string]any{"value": "dark"}, "Read setting theme.")
	// all keys: settings map present.
	runOK(t, runSettingsGet, `{}`,
		map[string]any{"settings": map[string]any{"a": 1, "b": 2}},
		"Read 2 setting(s).")
	// all keys: no settings map -> fallback.
	runOK(t, runSettingsGet, `{}`, map[string]any{}, "Read add-in settings.")
}

func TestRunListComments_Count(t *testing.T) {
	runOK(t, runListComments, `{"sheet":"S"}`,
		map[string]any{"comments": []any{map[string]any{}, map[string]any{}}},
		"Listed 2 comment(s).")
}

func TestRunListShapes_Count(t *testing.T) {
	runOK(t, runListShapes, `{"sheet":"S"}`,
		map[string]any{"shapes": []any{map[string]any{}}},
		"Listed 1 shape(s).")
}

func TestRunListCharts_Count(t *testing.T) {
	runOK(t, runListCharts, `{"sheet":"S"}`,
		map[string]any{"charts": []any{map[string]any{}, map[string]any{}}},
		"Listed 2 chart(s).")
}

func TestRunChartInfo_SummaryBranches(t *testing.T) {
	runOK(t, runChartInfo, `{"sheet":"S","name":"C1"}`,
		map[string]any{"chartType": "ColumnClustered"},
		"Chart C1 on S (type=ColumnClustered).")
	runOK(t, runChartInfo, `{"sheet":"S","name":"C1"}`,
		map[string]any{},
		"Returned info for chart C1 on S.")
}

func TestRunChartImage_WithDimensions(t *testing.T) {
	runOK(t, runChartImage, `{"sheet":"S","name":"C1","width":200,"height":100}`,
		map[string]any{"mimeType": "image/png"},
		"Rendered chart C1 on S as PNG.")
}

func TestRunPivotTableInfo_Summary(t *testing.T) {
	runOK(t, runPivotTableInfo, `{"name":"P1"}`, map[string]any{}, "Returned info for PivotTable P1.")
}

func TestRunPivotTableValues_Summary(t *testing.T) {
	runOK(t, runPivotTableValues, `{"name":"P1"}`,
		map[string]any{"address": "A1:D10", "rowCount": float64(10), "columnCount": float64(4)},
		"Read PivotTable P1 10x4 cells from A1:D10.")
}

func TestRunListPivotTables_Count(t *testing.T) {
	runOK(t, runListPivotTables, `{}`,
		map[string]any{"pivotTables": []any{map[string]any{}}},
		"Listed 1 PivotTable(s).")
}

func TestRunRunScript_WithArgs(t *testing.T) {
	runOK(t, runRunScript, `{"script":"return args.x;","scriptArgs":{"x":1}}`,
		map[string]any{"ok": true}, "Ran custom Excel.run script.")
}

func TestRunTabulateRegion_SummaryBranches(t *testing.T) {
	// happy with rows/cols and payload address.
	runOK(t, runTabulateRegion, `{"address":"A1:D200","sheet":"S","headers":"first_row","maxCells":5000}`,
		map[string]any{"address": "S!A1:D200", "rowCount": float64(200), "columnCount": float64(4)},
		"Tabulated S!A1:D200: 200 rows × 4 columns.")
	// truncated branch; payload omits address -> fallback to param.
	runOK(t, runTabulateRegion, `{"address":"A1:Z9999"}`,
		map[string]any{"truncated": true},
		"Region A1:Z9999 exceeds maxCells; not loaded.")
}

func TestRunApplyDiff_Branches(t *testing.T) {
	runOK(t, runApplyDiff, `{"patches":[{"address":"A1"},{"address":"B2"}]}`,
		map[string]any{"applied": []any{map[string]any{}, map[string]any{}}},
		"Applied 2 patch(es).")
	// empty patches array -> no_patches validation error, no attach.
	res := runApplyDiff(context.Background(), json.RawMessage(`{"patches":[]}`), errEnv())
	if res.Err == nil || res.Err.Code != "no_patches" {
		t.Fatalf("want no_patches, got %+v", res.Err)
	}
}

func TestRunSummarizeWorkbook_Summary(t *testing.T) {
	runOK(t, runSummarizeWorkbook, `{}`,
		map[string]any{
			"worksheets":  []any{map[string]any{}, map[string]any{}},
			"tables":      []any{map[string]any{}},
			"namedRanges": []any{},
		},
		"Workbook: 2 sheet(s), 1 table(s), 0 named range(s).")
}

func TestRunQuery_SummaryBranches(t *testing.T) {
	// normal count.
	runOK(t, runQuery, `{"address":"A1:F2000","sheet":"S","query":{"limit":10}}`,
		map[string]any{"count": float64(7)},
		"Query returned 7 row(s).")
	// limited.
	runOK(t, runQuery, `{"address":"A1:F2000"}`,
		map[string]any{"count": float64(10), "limited": true},
		"Query returned 10 row(s) (limited).")
	// truncated.
	runOK(t, runQuery, `{"address":"A1:Z99999","maxCells":100}`,
		map[string]any{"truncated": true},
		"Range A1:Z99999 exceeds maxCells; query not run.")
	// headers array passthrough does not break decode.
	runOK(t, runQuery, `{"address":"A1:B2","headers":["x","y"]}`,
		map[string]any{"count": float64(1)},
		"Query returned 1 row(s).")
}

// --- excel.discover (officetool.RunDiscover via runDiscover) ---------------

func discoverEnv(t *testing.T, store *doccache.Store, head any) *tools.RunEnv {
	t.Helper()
	env := fakeEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOffice(head), nil
	})
	env.DocCache = store
	return env
}

func TestRunDiscover_RefreshThenCacheHit(t *testing.T) {
	dir := t.TempDir()
	store := doccache.Open(filepath.Join(dir, "doccache.json"), false)
	payload := map[string]any{
		"filePath":    "Book1.xlsx",
		"fingerprint": "fp1",
		"worksheets":  []any{map[string]any{"name": "Sheet1"}},
	}

	// First call: cache miss -> refreshed + persisted.
	env := discoverEnv(t, store, payload)
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("refresh error: %+v", res.Err)
	}
	if res.Summary != "Excel discovery refreshed (Book1.xlsx)." {
		t.Errorf("refresh summary=%q", res.Summary)
	}
	m, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if m["cached"] != false || m["fingerprint"] != "fp1" {
		t.Errorf("meta=%v", m)
	}

	// Second call: same fingerprint -> cache hit.
	env2 := discoverEnv(t, store, payload)
	res2 := runDiscover(context.Background(), json.RawMessage(`{}`), env2)
	if res2.Err != nil {
		t.Fatalf("hit error: %+v", res2.Err)
	}
	if res2.Summary != "Excel discovery cache hit (Book1.xlsx)." {
		t.Errorf("hit summary=%q", res2.Summary)
	}
	if hm, _ := res2.Data.(map[string]any); hm["cached"] != true {
		t.Errorf("expected cached=true, got %v", res2.Data)
	}

	// Force bypass even on a fingerprint match -> refreshed again.
	env3 := discoverEnv(t, store, payload)
	res3 := runDiscover(context.Background(), json.RawMessage(`{"force":true}`), env3)
	if res3.Err != nil || res3.Summary != "Excel discovery refreshed (Book1.xlsx)." {
		t.Fatalf("force refresh got summary=%q err=%+v", res3.Summary, res3.Err)
	}
}

func TestRunDiscover_OfficeError(t *testing.T) {
	store := doccache.Open("", true) // disabled store: Get miss, Put no-op.
	env := fakeEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOfficeErr("ItemNotFound", "boom", nil), nil
	})
	env.DocCache = store
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "ItemNotFound" {
		t.Fatalf("want office_js/ItemNotFound, got %+v", res.Err)
	}
}

func TestRunDiscover_AttachFailure(t *testing.T) {
	env := errEnv()
	env.DocCache = doccache.Open("", true)
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunDiscover_ParamDecode(t *testing.T) {
	res := runDiscover(context.Background(), json.RawMessage(`{`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// --- direct helper coverage -------------------------------------------------

func TestHelpers_arrayLen(t *testing.T) {
	if got := arrayLen(map[string]any{"k": []any{1, 2, 3}}, "k"); got != 3 {
		t.Errorf("arrayLen=%d want 3", got)
	}
	if got := arrayLen("not a map", "k"); got != 0 {
		t.Errorf("non-map arrayLen=%d want 0", got)
	}
	if got := arrayLen(map[string]any{"k": "not array"}, "k"); got != 0 {
		t.Errorf("wrong-type arrayLen=%d want 0", got)
	}
	if got := arrayLen(map[string]any{}, "missing"); got != 0 {
		t.Errorf("missing arrayLen=%d want 0", got)
	}
}

func TestHelpers_stringField(t *testing.T) {
	if got := stringField(map[string]any{"k": "v"}, "k"); got != "v" {
		t.Errorf("stringField=%q want v", got)
	}
	if got := stringField("not a map", "k"); got != "" {
		t.Errorf("non-map stringField=%q want empty", got)
	}
	if got := stringField(map[string]any{"k": 42}, "k"); got != "" {
		t.Errorf("wrong-type stringField=%q want empty", got)
	}
}

func TestHelpers_boolField(t *testing.T) {
	if !boolField(map[string]any{"k": true}, "k") {
		t.Error("boolField true case failed")
	}
	if boolField("not a map", "k") {
		t.Error("non-map boolField should be false")
	}
	if boolField(map[string]any{"k": "true"}, "k") {
		t.Error("wrong-type boolField should be false")
	}
}

func TestHelpers_numberField(t *testing.T) {
	if got := numberField(map[string]any{"k": float64(7)}, "k"); got != 7 {
		t.Errorf("numberField float=%d want 7", got)
	}
	if got := numberField(map[string]any{"k": int(5)}, "k"); got != 5 {
		t.Errorf("numberField int=%d want 5", got)
	}
	if got := numberField("not a map", "k"); got != 0 {
		t.Errorf("non-map numberField=%d want 0", got)
	}
	if got := numberField(map[string]any{"k": "x"}, "k"); got != 0 {
		t.Errorf("wrong-type numberField=%d want 0", got)
	}
}

func TestSelectorFields_Selector(t *testing.T) {
	s := selectorFields{TargetID: "t-1", URLPattern: "taskpane"}
	sel := s.selector()
	if sel.TargetID != "t-1" || sel.URLPattern != "taskpane" {
		t.Errorf("selector=%+v", sel)
	}
}

// rangeReadSummary direct coverage for the bare-verb branch with no addr.
func TestRangeReadSummary_BareVerb(t *testing.T) {
	if got := rangeReadSummary(map[string]any{}, "Read", ""); got != "Read range." {
		t.Errorf("rangeReadSummary=%q want 'Read range.'", got)
	}
}

// Tool constructors return well-formed definitions; exercise them so the
// public surface is covered alongside the run* funcs.
func TestToolConstructors_NamesAndRun(t *testing.T) {
	ctors := []func() tools.Tool{
		ListWorksheets, GetActiveWorksheet, WorksheetInfo, ActivateWorksheet,
		CreateWorksheet, DeleteWorksheet, ReadRange, WriteRange, GetSelectedRange,
		SetSelectedRange, ActiveRange, UsedRange, RangeProperties, RangeFormulas,
		RangeSpecialCells, FindInRange, ListConditionalFormats, ListDataValidations,
		CreateTable, ListTables, TableInfo, TableRows, TableFilters, WorkbookInfo,
		CalculationState, ListNamedItems, CustomXMLParts, SettingsGet, ListComments,
		ListShapes, ListCharts, ChartInfo, ChartImage, ListPivotTables, PivotTableInfo,
		PivotTableValues, RunScript, TabulateRegion, ApplyDiff, SummarizeWorkbook,
		Query, Discover,
	}
	for _, ctor := range ctors {
		tool := ctor()
		if tool.Name == "" || tool.Run == nil || len(tool.Schema) == 0 {
			t.Errorf("malformed tool: name=%q runNil=%v schemaLen=%d", tool.Name, tool.Run == nil, len(tool.Schema))
		}
	}
}
