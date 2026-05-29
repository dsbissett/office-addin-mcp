package exceltool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const tabulateRegionSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.tabulateRegion parameters",
  "description": "Read a range and return it as a typed table (rows-as-objects keyed by inferred or supplied headers, plus per-column type tags).",
  "type": "object",
  "properties": {
    "address":  {"type": "string", "minLength": 1, "description": "Range address, e.g. 'Sheet1!A1:D200' or a defined name."},
    "sheet":    {"type": "string", "description": "Sheet name; default is the active sheet."},
    "headers":  {"type": "string", "enum": ["auto", "first_row", "none"], "default": "auto", "description": "How to derive column headers: 'first_row' = always treat row 1 as headers; 'none' = synthesize col1, col2, …; 'auto' = heuristic."},
    "maxCells": {"type": "integer", "minimum": 1, "default": 100000, "description": "Bail with truncated=true above this cell count instead of loading the whole grid."},` + targetSelectorBase + `},
  "required": ["address"],
  "additionalProperties": false
}`

type tabulateRegionParams struct {
	Address  string `json:"address"`
	Sheet    string `json:"sheet,omitempty"`
	Headers  string `json:"headers,omitempty"`
	MaxCells int    `json:"maxCells,omitempty"`
	selectorFields
}

// TabulateRegion returns the excel.tabulateRegion tool definition.
func TabulateRegion() tools.Tool {
	return tools.Tool{
		Name:        "excel.tabulateRegion",
		Description: "Load a range and return it as a typed table with inferred headers + per-column type tags. One call replaces readRange + manual header detection + per-cell type inspection.",
		Schema:      json.RawMessage(tabulateRegionSchema),
		Annotations: &tools.Annotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: tools.BoolPtr(false)},
		Run:         runTabulateRegion,
	}
}

func runTabulateRegion(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p tabulateRegionParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.tabulateRegion", p.args(), func(data any) string {
		return tabulateRegionSummary(data, p.Address)
	})
}

// args builds the Office.js argument map for excel.tabulateRegion.
func (p tabulateRegionParams) args() map[string]any {
	args := map[string]any{"address": p.Address}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	if p.Headers != "" {
		args["headers"] = p.Headers
	}
	if p.MaxCells > 0 {
		args["maxCells"] = p.MaxCells
	}
	return args
}

// tabulateRegionSummary describes the tabulated region's shape or truncation.
func tabulateRegionSummary(data any, fallbackAddr string) string {
	addr := addressOr(data, fallbackAddr)
	if boolField(data, "truncated") {
		return fmt.Sprintf("Region %s exceeds maxCells; not loaded.", addr)
	}
	rows := numberField(data, "rowCount")
	cols := numberField(data, "columnCount")
	return fmt.Sprintf("Tabulated %s: %d rows × %d columns.", addr, rows, cols)
}
