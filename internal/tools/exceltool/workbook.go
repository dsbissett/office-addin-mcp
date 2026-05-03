package exceltool

import (
	"context"
	"encoding/json"
	"fmt"

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
	return runPayloadSum(ctx, env, p.selector(), "excel.workbookInfo", map[string]any{}, func(data any) string {
		name := stringField(data, "name")
		if name == "" {
			return "Returned workbook info."
		}
		return "Returned workbook info for " + name + "."
	})
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
	return runPayloadSum(ctx, env, p.selector(), "excel.calculationState", map[string]any{}, func(data any) string {
		mode := stringField(data, "calculationMode")
		state := stringField(data, "calculationState")
		switch {
		case mode != "" && state != "":
			return fmt.Sprintf("Calculation mode=%s, state=%s.", mode, state)
		case mode != "":
			return "Calculation mode=" + mode + "."
		default:
			return "Returned calculation state."
		}
	})
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
	return runPayloadSum(ctx, env, p.selector(), "excel.listNamedItems", map[string]any{}, func(data any) string {
		return fmt.Sprintf("Listed %d named item(s).", arrayLen(data, "items"))
	})
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
	return runPayloadSum(ctx, env, p.selector(), "excel.customXmlParts", map[string]any{}, func(data any) string {
		return fmt.Sprintf("Listed %d custom XML part(s).", arrayLen(data, "parts"))
	})
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
	return runPayloadSum(ctx, env, p.selector(), "excel.settingsGet", args, func(data any) string {
		if p.Key != "" {
			return "Read setting " + p.Key + "."
		}
		if m, ok := data.(map[string]any); ok {
			if settings, ok := m["settings"].(map[string]any); ok {
				return fmt.Sprintf("Read %d setting(s).", len(settings))
			}
		}
		return "Read add-in settings."
	})
}
