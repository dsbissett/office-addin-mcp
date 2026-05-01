package exceltool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const listWorksheetsSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.listWorksheets parameters",
  "type": "object",
  "properties": {` + targetSelectorBase + `},
  "additionalProperties": false
}`

// ListWorksheets returns the excel.listWorksheets tool definition.
func ListWorksheets() tools.Tool {
	return tools.Tool{
		Name:        "excel.listWorksheets",
		Description: "List all worksheets in the active workbook with name, id, position, and visibility.",
		Schema:      json.RawMessage(listWorksheetsSchema),
		Run:         runListWorksheets,
	}
}

func runListWorksheets(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.listWorksheets", map[string]any{})
}

const getActiveWorksheetSchema = listWorksheetsSchema

// GetActiveWorksheet returns the excel.getActiveWorksheet tool definition.
func GetActiveWorksheet() tools.Tool {
	return tools.Tool{
		Name:        "excel.getActiveWorksheet",
		Description: "Return the active worksheet's name, id, position, and visibility.",
		Schema:      json.RawMessage(getActiveWorksheetSchema),
		Run:         runGetActiveWorksheet,
	}
}

func runGetActiveWorksheet(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.getActiveWorksheet", map[string]any{})
}

const worksheetInfoSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.worksheetInfo parameters",
  "type": "object",
  "properties": {
    "sheet": {"type": "string", "description": "Worksheet name. Omit to use the active worksheet."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

type optionalSheetParams struct {
	Sheet string `json:"sheet,omitempty"`
	selectorFields
}

// WorksheetInfo returns the excel.worksheetInfo tool definition.
func WorksheetInfo() tools.Tool {
	return tools.Tool{
		Name:        "excel.worksheetInfo",
		Description: "Metadata for a single worksheet: used range address, visibility, protection, gridlines, tab color, and dimensions.",
		Schema:      json.RawMessage(worksheetInfoSchema),
		Run:         runWorksheetInfo,
	}
}

func runWorksheetInfo(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p optionalSheetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	return runPayload(ctx, env, p.selector(), "excel.worksheetInfo", args)
}

const namedWorksheetSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "named worksheet parameters",
  "type": "object",
  "properties": {
    "name": {"type": "string", "minLength": 1},` + targetSelectorBase + `},
  "required": ["name"],
  "additionalProperties": false
}`

type namedWorksheetParams struct {
	Name string `json:"name"`
	selectorFields
}

// ActivateWorksheet returns the excel.activateWorksheet tool definition.
func ActivateWorksheet() tools.Tool {
	return tools.Tool{
		Name:        "excel.activateWorksheet",
		Description: "Activate a worksheet by name. Requires ExcelApi 1.7.",
		Schema:      json.RawMessage(namedWorksheetSchema),
		Run:         runActivateWorksheet,
	}
}

func runActivateWorksheet(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p namedWorksheetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.activateWorksheet", map[string]any{"name": p.Name})
}

// CreateWorksheet returns the excel.createWorksheet tool definition.
func CreateWorksheet() tools.Tool {
	return tools.Tool{
		Name:        "excel.createWorksheet",
		Description: "Add a new worksheet to the workbook with the given name.",
		Schema:      json.RawMessage(namedWorksheetSchema),
		Run:         runCreateWorksheet,
	}
}

func runCreateWorksheet(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p namedWorksheetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.createWorksheet", map[string]any{"name": p.Name})
}

// DeleteWorksheet returns the excel.deleteWorksheet tool definition.
func DeleteWorksheet() tools.Tool {
	return tools.Tool{
		Name:        "excel.deleteWorksheet",
		Description: "Delete a worksheet by name. The active or last visible sheet may be protected by Excel.",
		Schema:      json.RawMessage(namedWorksheetSchema),
		Run:         runDeleteWorksheet,
	}
}

func runDeleteWorksheet(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p namedWorksheetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.deleteWorksheet", map[string]any{"name": p.Name})
}
