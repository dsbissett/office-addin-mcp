package exceltool

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// TestAnnotations_RepresentativeTools asserts the annotation classification on a
// spread of read-only, additive, and destructive tools so a misclassification
// (e.g. flipping a read-only tool's DestructiveHint) fails the build.
func TestAnnotations_RepresentativeTools(t *testing.T) {
	readOnly := []func() tools.Tool{
		ReadRange, GetSelectedRange, ActiveRange, UsedRange, ListWorksheets,
		WorkbookInfo, ListTables, TableRows, ListCharts, ChartImage,
		TabulateRegion, SummarizeWorkbook, Query, Discover,
	}
	for _, ctor := range readOnly {
		tool := ctor()
		a := tool.Annotations
		if a == nil {
			t.Fatalf("%s: nil annotations", tool.Name)
		}
		if !a.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint=false, want true", tool.Name)
		}
		if !a.IdempotentHint {
			t.Errorf("%s: IdempotentHint=false, want true", tool.Name)
		}
		if a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("%s: DestructiveHint=%v, want *false", tool.Name, a.DestructiveHint)
		}
	}

	// Destructive: overwrite/delete/replace/arbitrary-code.
	destructive := []func() tools.Tool{
		WriteRange, ApplyDiff, SetSelectedRange, DeleteWorksheet, RunScript,
	}
	for _, ctor := range destructive {
		tool := ctor()
		a := tool.Annotations
		if a == nil {
			t.Fatalf("%s: nil annotations", tool.Name)
		}
		if a.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint=true, want false", tool.Name)
		}
		if a.DestructiveHint == nil || !*a.DestructiveHint {
			t.Errorf("%s: DestructiveHint=%v, want *true", tool.Name, a.DestructiveHint)
		}
	}

	// Additive mutations: not read-only, not destructive.
	additive := []func() tools.Tool{
		CreateWorksheet, CreateTable, ActivateWorksheet,
	}
	for _, ctor := range additive {
		tool := ctor()
		a := tool.Annotations
		if a == nil {
			t.Fatalf("%s: nil annotations", tool.Name)
		}
		if a.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint=true, want false", tool.Name)
		}
		if a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("%s: DestructiveHint=%v, want *false", tool.Name, a.DestructiveHint)
		}
	}

	// runScript reaches arbitrary/external entities -> OpenWorldHint true.
	if a := RunScript().Annotations; a.OpenWorldHint == nil || !*a.OpenWorldHint {
		t.Errorf("runScript OpenWorldHint=%v, want *true", a.OpenWorldHint)
	}
}

// TestEveryToolHasAnnotations guards against a future constructor being added
// without annotations.
func TestEveryToolHasAnnotations(t *testing.T) {
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
		if tool.Annotations == nil {
			t.Errorf("%s: missing Annotations", tool.Name)
		}
	}
}

// compileOutputSchema compiles an OutputSchema blob with the same library the
// dispatcher uses; a malformed schema fails here.
func compileOutputSchema(t *testing.T, name string, raw json.RawMessage) *jsonschema.Schema {
	t.Helper()
	if len(bytes.TrimSpace(raw)) == 0 {
		t.Fatalf("%s: empty OutputSchema", name)
	}
	c := jsonschema.NewCompiler()
	url := "mem://" + name + ".out.schema.json"
	if err := c.AddResource(url, bytes.NewReader(raw)); err != nil {
		t.Fatalf("%s: add resource: %v", name, err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		t.Fatalf("%s: compile: %v", name, err)
	}
	return sch
}

// validateAgainst unmarshals a representative payload and validates it against
// the compiled schema, failing the test on a mismatch.
func validateAgainst(t *testing.T, name string, sch *jsonschema.Schema, payload string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		t.Fatalf("%s: payload not JSON: %v", name, err)
	}
	if err := sch.Validate(v); err != nil {
		t.Errorf("%s: representative payload rejected: %v", name, err)
	}
}

// TestQueryOutputSchema_ValidatesPayloads compiles excel.query's OutputSchema
// and validates both the loaded-grid and truncated success payloads against it.
func TestQueryOutputSchema_ValidatesPayloads(t *testing.T) {
	tool := Query()
	sch := compileOutputSchema(t, tool.Name, tool.OutputSchema)
	// Loaded-grid success path.
	validateAgainst(t, tool.Name, sch, `{
		"address": "Sheet1!A1:F2000",
		"rowCount": 2000,
		"columnCount": 6,
		"headers": ["id", "name", "qty"],
		"truncated": false,
		"rows": [{"id": 1, "name": "a"}],
		"count": 1,
		"limited": false
	}`)
	// Truncated bail-out path (rows null, no headers/limited).
	validateAgainst(t, tool.Name, sch, `{
		"address": "Sheet1!A1:Z99999",
		"rowCount": 99999,
		"columnCount": 26,
		"truncated": true,
		"rows": null,
		"count": 0
	}`)
}

// TestDiscoverOutputSchema_ValidatesPayload compiles excel.discover's
// OutputSchema and validates a representative refreshed payload (JS payload plus
// the cache metadata injected by officetool.withCacheMeta).
func TestDiscoverOutputSchema_ValidatesPayload(t *testing.T) {
	tool := Discover()
	sch := compileOutputSchema(t, tool.Name, tool.OutputSchema)
	validateAgainst(t, tool.Name, sch, `{
		"cached": false,
		"filePath": "Book1.xlsx",
		"fingerprint": "wb:2:t1:n0:c150",
		"worksheets": [{"name": "Sheet1", "id": "s1", "position": 0, "visibility": "Visible", "usedRange": {"address": "A1:C10", "rowCount": 10, "columnCount": 3}}],
		"tables": [{"name": "T1", "id": "t1", "sheet": "Sheet1", "showHeaders": true, "showTotals": false}],
		"namedRanges": [{"name": "MyName", "type": "Range", "value": "Sheet1!$A$1", "scope": "Workbook"}]
	}`)
}
