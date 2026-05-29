package exceltool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const querySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.query parameters",
  "description": "Server-side range query: load values, project into row objects, then filter/groupBy/agg/limit before returning. Replaces 'read 100k rows then filter in agent context'.",
  "type": "object",
  "properties": {
    "address":  {"type": "string", "minLength": 1, "description": "Range address, e.g. 'Sheet1!A1:F2000'."},
    "sheet":    {"type": "string"},
    "headers":  {
      "oneOf": [
        {"type": "string", "enum": ["first_row", "none"]},
        {"type": "array", "items": {"type": "string"}, "description": "Explicit header names; bypasses inference."}
      ],
      "description": "How to derive column headers. Default: 'first_row'."
    },
    "maxCells": {"type": "integer", "minimum": 1, "default": 500000, "description": "Bail with truncated=true above this cell count instead of loading the grid."},
    "query": {
      "type": "object",
      "description": "JSON-shaped query. All keys optional; an empty object returns all rows up to limit.",
      "properties": {
        "filter":  {"description": "Filter predicate: {op:[args]}. Ops: ==,!=,<,<=,>,>=,and,or,not,in,contains,var. String args resolve as field names when present in the row."},
        "project": {"type": "array", "items": {"type": "string"}, "description": "Restrict output rows to these columns."},
        "groupBy": {"type": "array", "items": {"type": "string"}, "description": "Group rows by these columns; combine with agg."},
        "agg":     {"type": "array", "items": {"type": "object", "properties": {"col": {"type": "string"}, "fn": {"type": "string", "enum": ["sum","count","avg","min","max"]}, "as": {"type": "string"}}, "required": ["col","fn"], "additionalProperties": false}, "description": "Aggregations applied per group (or globally when groupBy is omitted)."},
        "limit":   {"type": "integer", "minimum": 1, "description": "Cap output row count."}
      },
      "additionalProperties": false
    },` + targetSelectorBase + `},
  "required": ["address"],
  "additionalProperties": false
}`

// queryOutputSchema describes the success Data shape returned by excel.query.
// Top-level properties match the excel_query.js payload (both the loaded-grid
// and the truncated bail-out paths); nested row objects are left permissive.
const queryOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.query result",
  "type": "object",
  "properties": {
    "address":     {"type": "string"},
    "rowCount":    {"type": "integer"},
    "columnCount": {"type": "integer"},
    "headers":     {"type": "array", "items": {"type": "string"}},
    "truncated":   {"type": "boolean"},
    "rows":        {"type": ["array", "null"]},
    "count":       {"type": "integer"},
    "limited":     {"type": "boolean"}
  },
  "required": ["address", "rowCount", "columnCount", "truncated", "count"]
}`

type queryParams struct {
	Address  string          `json:"address"`
	Sheet    string          `json:"sheet,omitempty"`
	Headers  json.RawMessage `json:"headers,omitempty"`
	MaxCells int             `json:"maxCells,omitempty"`
	Query    json.RawMessage `json:"query,omitempty"`
	selectorFields
}

// Query returns the excel.query tool definition.
func Query() tools.Tool {
	return tools.Tool{
		Name:         "excel.query",
		Description:  "Run a JSON-shaped filter/project/groupBy/agg query against an Excel range. Returns a small summarized answer instead of the raw grid.",
		Schema:       json.RawMessage(querySchema),
		OutputSchema: json.RawMessage(queryOutputSchema),
		Annotations:  &tools.Annotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: tools.BoolPtr(false)},
		Run:          runQuery,
	}
}

func runQuery(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p queryParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.query", p.args(), func(data any) string {
		return querySummary(data, p.Address)
	})
}

// args builds the Office.js argument map for excel.query.
func (p queryParams) args() map[string]any {
	args := map[string]any{"address": p.Address}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	setRawArg(args, "headers", p.Headers)
	if p.MaxCells > 0 {
		args["maxCells"] = p.MaxCells
	}
	setRawArg(args, "query", p.Query)
	return args
}

// querySummary describes the query result row count or a truncation notice.
func querySummary(data any, address string) string {
	if boolField(data, "truncated") {
		return fmt.Sprintf("Range %s exceeds maxCells; query not run.", address)
	}
	count := numberField(data, "count")
	if boolField(data, "limited") {
		return fmt.Sprintf("Query returned %d row(s) (limited).", count)
	}
	return fmt.Sprintf("Query returned %d row(s).", count)
}
