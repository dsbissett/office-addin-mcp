package exceltool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const readRangeSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.readRange parameters",
  "type": "object",
  "properties": {
    "address": {"type": "string", "minLength": 1, "description": "Range address, e.g. 'A1:D10' or a defined name."},
    "sheet":   {"type": "string", "description": "Sheet name; default is the active sheet."},` + targetSelectorBase + `},
  "required": ["address"],
  "additionalProperties": false
}`

type readRangeParams struct {
	Address string `json:"address"`
	Sheet   string `json:"sheet,omitempty"`
	selectorFields
}

// ReadRange returns the excel.readRange tool definition.
func ReadRange() tools.Tool {
	return tools.Tool{
		Name:        "excel.readRange",
		Description: "Read values, formulas, and number formats of a range. Defaults to the active worksheet.",
		Schema:      json.RawMessage(readRangeSchema),
		Run:         runReadRange,
	}
}

func runReadRange(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p readRangeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{"address": p.Address}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	return runPayload(ctx, env, p.selector(), "excel.readRange", args)
}

const writeRangeSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.writeRange parameters",
  "type": "object",
  "properties": {
    "address":      {"type": "string", "minLength": 1},
    "sheet":        {"type": "string"},
    "values":       {"type": "array", "items": {"type": "array"}, "description": "2-D array of values shaped like the range."},
    "formulas":     {"type": "array", "items": {"type": "array"}, "description": "2-D array of formulas; takes precedence over values for cells where both are set."},
    "numberFormat": {
      "oneOf": [
        {"type": "string"},
        {"type": "array", "items": {"type": "array"}}
      ]
    },` + targetSelectorBase + `},
  "required": ["address"],
  "anyOf": [
    {"required": ["values"]},
    {"required": ["formulas"]},
    {"required": ["numberFormat"]}
  ],
  "additionalProperties": false
}`

type writeRangeParams struct {
	Address      string          `json:"address"`
	Sheet        string          `json:"sheet,omitempty"`
	Values       json.RawMessage `json:"values,omitempty"`
	Formulas     json.RawMessage `json:"formulas,omitempty"`
	NumberFormat json.RawMessage `json:"numberFormat,omitempty"`
	selectorFields
}

// WriteRange returns the excel.writeRange tool definition.
func WriteRange() tools.Tool {
	return tools.Tool{
		Name:        "excel.writeRange",
		Description: "Write values, formulas, or number formats to a range. At least one of values/formulas/numberFormat is required.",
		Schema:      json.RawMessage(writeRangeSchema),
		Run:         runWriteRange,
	}
}

func runWriteRange(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p writeRangeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{"address": p.Address}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	if len(p.Values) > 0 {
		args["values"] = json.RawMessage(p.Values)
	}
	if len(p.Formulas) > 0 {
		args["formulas"] = json.RawMessage(p.Formulas)
	}
	if len(p.NumberFormat) > 0 {
		args["numberFormat"] = json.RawMessage(p.NumberFormat)
	}
	return runPayload(ctx, env, p.selector(), "excel.writeRange", args)
}

const getSelectedRangeSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.getSelectedRange parameters",
  "type": "object",
  "properties": {` + targetSelectorBase + `},
  "additionalProperties": false
}`

type emptySelectorParams struct {
	selectorFields
}

// GetSelectedRange returns the excel.getSelectedRange tool definition.
func GetSelectedRange() tools.Tool {
	return tools.Tool{
		Name:        "excel.getSelectedRange",
		Description: "Return the address, values, and shape of the currently selected range.",
		Schema:      json.RawMessage(getSelectedRangeSchema),
		Run:         runGetSelectedRange,
	}
}

func runGetSelectedRange(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.getSelectedRange", map[string]any{})
}

const setSelectedRangeSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.setSelectedRange parameters",
  "type": "object",
  "properties": {
    "address": {"type": "string", "minLength": 1},
    "sheet":   {"type": "string"},` + targetSelectorBase + `},
  "required": ["address"],
  "additionalProperties": false
}`

// SetSelectedRange returns the excel.setSelectedRange tool definition.
func SetSelectedRange() tools.Tool {
	return tools.Tool{
		Name:        "excel.setSelectedRange",
		Description: "Select a range by address, optionally on a named sheet.",
		Schema:      json.RawMessage(setSelectedRangeSchema),
		Run:         runSetSelectedRange,
	}
}

func runSetSelectedRange(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p struct {
		Address string `json:"address"`
		Sheet   string `json:"sheet,omitempty"`
		selectorFields
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{"address": p.Address}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	return runPayload(ctx, env, p.selector(), "excel.setSelectedRange", args)
}
