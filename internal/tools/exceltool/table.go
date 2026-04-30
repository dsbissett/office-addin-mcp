package exceltool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const createTableSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.createTable parameters",
  "type": "object",
  "properties": {
    "address":    {"type": "string", "minLength": 1, "description": "Range address that becomes the table body."},
    "sheet":      {"type": "string"},
    "name":       {"type": "string", "description": "Optional table name. Excel auto-generates one if omitted."},
    "hasHeaders": {"type": "boolean", "description": "Whether the first row contains headers (default true)."},` + targetSelectorBase + `},
  "required": ["address"],
  "additionalProperties": false
}`

type createTableParams struct {
	Address    string `json:"address"`
	Sheet      string `json:"sheet,omitempty"`
	Name       string `json:"name,omitempty"`
	HasHeaders *bool  `json:"hasHeaders,omitempty"`
	selectorFields
}

// CreateTable returns the excel.createTable tool definition.
func CreateTable() tools.Tool {
	return tools.Tool{
		Name:        "excel.createTable",
		Description: "Convert a range into an Excel table.",
		Schema:      json.RawMessage(createTableSchema),
		Run:         runCreateTable,
	}
}

func runCreateTable(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p createTableParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{"address": p.Address}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	if p.Name != "" {
		args["name"] = p.Name
	}
	if p.HasHeaders != nil {
		args["hasHeaders"] = *p.HasHeaders
	}
	return runPayload(ctx, env, p.selector(), "excel.createTable", args)
}
