package exceltool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const listPivotTablesSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.listPivotTables parameters",
  "type": "object",
  "properties": {` + targetSelectorBase + `},
  "additionalProperties": false
}`

// ListPivotTables returns the excel.listPivotTables tool definition.
func ListPivotTables() tools.Tool {
	return tools.Tool{
		Name:        "excel.listPivotTables",
		Description: "List all PivotTables in the workbook with name, worksheet, layout address, and enabled flags.",
		Schema:      json.RawMessage(listPivotTablesSchema),
		Run:         runListPivotTables,
	}
}

func runListPivotTables(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.listPivotTables", map[string]any{}, func(data any) string {
		return fmt.Sprintf("Listed %d PivotTable(s).", arrayLen(data, "pivotTables"))
	})
}

const namedPivotSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "named PivotTable parameters",
  "type": "object",
  "properties": {
    "name": {"type": "string", "minLength": 1, "description": "PivotTable name."},` + targetSelectorBase + `},
  "required": ["name"],
  "additionalProperties": false
}`

type namedPivotParams struct {
	Name string `json:"name"`
	selectorFields
}

// PivotTableInfo returns the excel.pivotTableInfo tool definition.
func PivotTableInfo() tools.Tool {
	return tools.Tool{
		Name:        "excel.pivotTableInfo",
		Description: "Structure of a PivotTable: row, column, data, and filter hierarchies with their source field names.",
		Schema:      json.RawMessage(namedPivotSchema),
		Run:         runPivotTableInfo,
	}
}

func runPivotTableInfo(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p namedPivotParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.pivotTableInfo", map[string]any{"name": p.Name}, func(_ any) string {
		return "Returned info for PivotTable " + p.Name + "."
	})
}

// PivotTableValues returns the excel.pivotTableValues tool definition.
func PivotTableValues() tools.Tool {
	return tools.Tool{
		Name:        "excel.pivotTableValues",
		Description: "Rendered values of a PivotTable layout range, truncated when it exceeds the cell cap.",
		Schema:      json.RawMessage(namedPivotSchema),
		Run:         runPivotTableValues,
	}
}

func runPivotTableValues(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p namedPivotParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.pivotTableValues", map[string]any{
		"name":     p.Name,
		"maxCells": maxCells,
	}, func(data any) string {
		return rangeReadSummary(data, "Read PivotTable "+p.Name, "")
	})
}
