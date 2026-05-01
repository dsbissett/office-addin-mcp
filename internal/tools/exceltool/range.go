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
	if len(p.Values) == 0 && len(p.Formulas) == 0 && len(p.NumberFormat) == 0 {
		return tools.Fail(tools.CategoryValidation, "missing_payload", "provide at least one of: values, formulas, numberFormat", false)
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

const activeRangeSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.activeRange parameters",
  "type": "object",
  "properties": {
    "includeFormulas":     {"type": "boolean", "description": "Include A1-style formulas per cell."},
    "includeNumberFormat": {"type": "boolean", "description": "Include the Excel number-format code per cell."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

type activeRangeParams struct {
	IncludeFormulas     bool `json:"includeFormulas,omitempty"`
	IncludeNumberFormat bool `json:"includeNumberFormat,omitempty"`
	selectorFields
}

// ActiveRange returns the excel.activeRange tool definition.
func ActiveRange() tools.Tool {
	return tools.Tool{
		Name:        "excel.activeRange",
		Description: "Currently selected Excel range with values + dimensions. Optional formulas / number formats. Truncates oversized payloads to the top-left cell.",
		Schema:      json.RawMessage(activeRangeSchema),
		Run:         runActiveRange,
	}
}

func runActiveRange(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p activeRangeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{
		"includeFormulas":     p.IncludeFormulas,
		"includeNumberFormat": p.IncludeNumberFormat,
		"maxCells":            maxCells,
	}
	return runPayload(ctx, env, p.selector(), "excel.activeRange", args)
}

const usedRangeSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.usedRange parameters",
  "type": "object",
  "properties": {
    "sheet":               {"type": "string", "description": "Worksheet name. Omit to use the active worksheet."},
    "valuesOnly":          {"type": "boolean", "description": "If true (default), only cells with values count toward the used range."},
    "includeFormulas":     {"type": "boolean"},
    "includeNumberFormat": {"type": "boolean"},` + targetSelectorBase + `},
  "additionalProperties": false
}`

type usedRangeParams struct {
	Sheet               string `json:"sheet,omitempty"`
	ValuesOnly          *bool  `json:"valuesOnly,omitempty"`
	IncludeFormulas     bool   `json:"includeFormulas,omitempty"`
	IncludeNumberFormat bool   `json:"includeNumberFormat,omitempty"`
	selectorFields
}

// UsedRange returns the excel.usedRange tool definition.
func UsedRange() tools.Tool {
	return tools.Tool{
		Name:        "excel.usedRange",
		Description: "Values (and optionally formulas / number formats) for a worksheet's used range, truncated when it exceeds the cell cap.",
		Schema:      json.RawMessage(usedRangeSchema),
		Run:         runUsedRange,
	}
}

func runUsedRange(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p usedRangeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	valuesOnly := true
	if p.ValuesOnly != nil {
		valuesOnly = *p.ValuesOnly
	}
	args := map[string]any{
		"valuesOnly":          valuesOnly,
		"includeFormulas":     p.IncludeFormulas,
		"includeNumberFormat": p.IncludeNumberFormat,
		"maxCells":            maxCells,
	}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	return runPayload(ctx, env, p.selector(), "excel.usedRange", args)
}

const rangeTargetFields = `
    "address": {"type": "string", "description": "A1 reference such as 'Sheet1!A1:C10' or 'A1:C10'. If omitted, the active selection is used."},
    "sheet":   {"type": "string", "description": "Worksheet name. Used when address omits the sheet prefix; ignored if address includes one."}
`

type rangeTargetParams struct {
	Address string `json:"address,omitempty"`
	Sheet   string `json:"sheet,omitempty"`
	selectorFields
}

func (p rangeTargetParams) baseArgs() map[string]any {
	args := map[string]any{}
	if p.Address != "" {
		args["address"] = p.Address
	}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	return args
}

const rangePropertiesSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.rangeProperties parameters",
  "type": "object",
  "properties": {` + rangeTargetFields + `,
    "includeFormat": {"type": "boolean", "description": "Include font, fill, alignment, and border summary per cell."},
    "includeStyle":  {"type": "boolean", "description": "Include the named style of each cell."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

type rangePropertiesParams struct {
	rangeTargetParams
	IncludeFormat bool `json:"includeFormat,omitempty"`
	IncludeStyle  bool `json:"includeStyle,omitempty"`
}

// RangeProperties returns the excel.rangeProperties tool definition.
func RangeProperties() tools.Tool {
	return tools.Tool{
		Name:        "excel.rangeProperties",
		Description: "Range properties: value types, hasSpill, row/column hidden flags, optional format (font/fill/alignment) and style.",
		Schema:      json.RawMessage(rangePropertiesSchema),
		Run:         runRangeProperties,
	}
}

func runRangeProperties(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p rangePropertiesParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := p.baseArgs()
	args["includeFormat"] = p.IncludeFormat
	args["includeStyle"] = p.IncludeStyle
	args["maxCells"] = maxCells
	return runPayload(ctx, env, p.selector(), "excel.rangeProperties", args)
}

const rangeFormulasSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.rangeFormulas parameters",
  "type": "object",
  "properties": {` + rangeTargetFields + `,` + targetSelectorBase + `},
  "additionalProperties": false
}`

// RangeFormulas returns the excel.rangeFormulas tool definition.
func RangeFormulas() tools.Tool {
	return tools.Tool{
		Name:        "excel.rangeFormulas",
		Description: "Formulas (A1 and R1C1) and resolved values for a range. Useful for verifying formula edits.",
		Schema:      json.RawMessage(rangeFormulasSchema),
		Run:         runRangeFormulas,
	}
}

func runRangeFormulas(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p rangeTargetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := p.baseArgs()
	args["maxCells"] = maxCells
	return runPayload(ctx, env, p.selector(), "excel.rangeFormulas", args)
}

const rangeSpecialCellsSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.rangeSpecialCells parameters",
  "type": "object",
  "properties": {` + rangeTargetFields + `,
    "cellType":  {"type": "string", "enum": ["constants", "formulas", "blanks", "visible"], "description": "Category of special cells to locate."},
    "valueType": {"type": "string", "enum": ["all", "errors", "logical", "numbers", "text"], "description": "For 'constants' or 'formulas', filter by value type. Defaults to 'all'."},` + targetSelectorBase + `},
  "required": ["cellType"],
  "additionalProperties": false
}`

type rangeSpecialCellsParams struct {
	rangeTargetParams
	CellType  string `json:"cellType"`
	ValueType string `json:"valueType,omitempty"`
}

// RangeSpecialCells returns the excel.rangeSpecialCells tool definition.
func RangeSpecialCells() tools.Tool {
	return tools.Tool{
		Name:        "excel.rangeSpecialCells",
		Description: "Locate special cells inside a range: constants, formulas, blanks, or visible. Returns matching address and cell count.",
		Schema:      json.RawMessage(rangeSpecialCellsSchema),
		Run:         runRangeSpecialCells,
	}
}

func runRangeSpecialCells(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p rangeSpecialCellsParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := p.baseArgs()
	args["cellType"] = p.CellType
	if p.ValueType != "" {
		args["valueType"] = p.ValueType
	}
	return runPayload(ctx, env, p.selector(), "excel.rangeSpecialCells", args)
}

const findInRangeSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.findInRange parameters",
  "type": "object",
  "properties": {` + rangeTargetFields + `,
    "text":          {"type": "string", "description": "Text to search for."},
    "completeMatch": {"type": "boolean", "description": "Require a whole-cell match. Defaults to false."},
    "matchCase":     {"type": "boolean", "description": "Case-sensitive match. Defaults to false."},` + targetSelectorBase + `},
  "required": ["text"],
  "additionalProperties": false
}`

type findInRangeParams struct {
	rangeTargetParams
	Text          string `json:"text"`
	CompleteMatch bool   `json:"completeMatch,omitempty"`
	MatchCase     bool   `json:"matchCase,omitempty"`
}

// FindInRange returns the excel.findInRange tool definition.
func FindInRange() tools.Tool {
	return tools.Tool{
		Name:        "excel.findInRange",
		Description: "Find all matches of a text string within a range. Returns the combined match address and cell count.",
		Schema:      json.RawMessage(findInRangeSchema),
		Run:         runFindInRange,
	}
}

func runFindInRange(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p findInRangeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := p.baseArgs()
	args["text"] = p.Text
	args["completeMatch"] = p.CompleteMatch
	args["matchCase"] = p.MatchCase
	return runPayload(ctx, env, p.selector(), "excel.findInRange", args)
}

const listConditionalFormatsSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.listConditionalFormats parameters",
  "type": "object",
  "properties": {` + rangeTargetFields + `,` + targetSelectorBase + `},
  "additionalProperties": false
}`

// ListConditionalFormats returns the excel.listConditionalFormats tool definition.
func ListConditionalFormats() tools.Tool {
	return tools.Tool{
		Name:        "excel.listConditionalFormats",
		Description: "List conditional-format rules on a range. Omit address to use the active worksheet's used range.",
		Schema:      json.RawMessage(listConditionalFormatsSchema),
		Run:         runListConditionalFormats,
	}
}

func runListConditionalFormats(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p rangeTargetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.listConditionalFormats", p.baseArgs())
}

const listDataValidationsSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.listDataValidations parameters",
  "type": "object",
  "properties": {` + rangeTargetFields + `,` + targetSelectorBase + `},
  "additionalProperties": false
}`

// ListDataValidations returns the excel.listDataValidations tool definition.
func ListDataValidations() tools.Tool {
	return tools.Tool{
		Name:        "excel.listDataValidations",
		Description: "Data-validation configuration on a range: type, rule, error alert, prompt. Omit address to use the active selection.",
		Schema:      json.RawMessage(listDataValidationsSchema),
		Run:         runListDataValidations,
	}
}

func runListDataValidations(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p rangeTargetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayload(ctx, env, p.selector(), "excel.listDataValidations", p.baseArgs())
}
