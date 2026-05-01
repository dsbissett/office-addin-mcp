package exceltool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const workbookInfoSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.workbookInfo parameters",
  "type": "object",
  "properties": {` + targetSelectorBase + `},
  "additionalProperties": false
}`

// WorkbookInfo returns the excel.workbookInfo tool definition.
func WorkbookInfo() tools.Tool {
	return tools.Tool{
		Name:        "excel.workbookInfo",
		Description: "Workbook-level metadata: name, save state, calculation mode/state, and protection state.",
		Schema:      json.RawMessage(workbookInfoSchema),
		Run:         runWorkbookInfo,
	}
}

func runWorkbookInfo(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.workbookInfo", map[string]any{})
}

// CalculationState returns the excel.calculationState tool definition.
func CalculationState() tools.Tool {
	return tools.Tool{
		Name:        "excel.calculationState",
		Description: "Workbook calculation mode (automatic/manual/etc.) and current calculation state.",
		Schema:      json.RawMessage(workbookInfoSchema),
		Run:         runCalculationState,
	}
}

func runCalculationState(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.calculationState", map[string]any{})
}

// ListNamedItems returns the excel.listNamedItems tool definition.
func ListNamedItems() tools.Tool {
	return tools.Tool{
		Name:        "excel.listNamedItems",
		Description: "List workbook-scoped named items (named ranges and formulas) with name, type, value, formula, visibility, and comment.",
		Schema:      json.RawMessage(workbookInfoSchema),
		Run:         runListNamedItems,
	}
}

func runListNamedItems(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.listNamedItems", map[string]any{})
}

// CustomXMLParts returns the excel.customXmlParts tool definition.
func CustomXMLParts() tools.Tool {
	return tools.Tool{
		Name:        "excel.customXmlParts",
		Description: "List custom XML parts stored in the workbook: id and namespace URI.",
		Schema:      json.RawMessage(workbookInfoSchema),
		Run:         runCustomXMLParts,
	}
}

func runCustomXMLParts(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.customXmlParts", map[string]any{})
}

const settingsGetSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.settingsGet parameters",
  "type": "object",
  "properties": {
    "key": {"type": "string", "description": "If provided, return only this setting's value. Otherwise, return all settings."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

type settingsGetParams struct {
	Key string `json:"key,omitempty"`
	selectorFields
}

// SettingsGet returns the excel.settingsGet tool definition.
func SettingsGet() tools.Tool {
	return tools.Tool{
		Name:        "excel.settingsGet",
		Description: "Read add-in document settings from Office.context.document.settings. Returns all keys or a single key's value.",
		Schema:      json.RawMessage(settingsGetSchema),
		Run:         runSettingsGet,
	}
}

func runSettingsGet(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p settingsGetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{}
	if p.Key != "" {
		args["key"] = p.Key
	}
	return runPayload(ctx, env, p.selector(), "excel.settingsGet", args)
}
