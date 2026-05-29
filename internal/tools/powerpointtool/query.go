package powerpointtool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const querySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "powerpoint.query parameters",
  "description": "Run a JSON-shaped filter/project/groupBy/agg query against the presentation's slide+shape catalog. Records: {slideId, slideIndex, shapeId, name, type, left, top, width, height}.",
  "type": "object",
  "properties": {
    "query": {
      "type": "object",
      "properties": {
        "filter":  {"description": "Filter predicate; same DSL as excel.query.query.filter."},
        "project": {"type": "array", "items": {"type": "string"}},
        "groupBy": {"type": "array", "items": {"type": "string"}},
        "agg":     {"type": "array", "items": {"type": "object", "properties": {"col": {"type": "string"}, "fn": {"type": "string", "enum": ["sum","count","avg","min","max"]}, "as": {"type": "string"}}, "required": ["col","fn"], "additionalProperties": false}},
        "limit":   {"type": "integer", "minimum": 1}
      },
      "additionalProperties": false
    },` + targetSelectorBase + `},
  "additionalProperties": false
}`

const queryOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "powerpoint.query result",
  "type": "object",
  "required": ["slideCount", "shapeCount", "rows", "count", "limited"],
  "properties": {
    "slideCount": {"type": "integer", "minimum": 0, "description": "Total slides scanned."},
    "shapeCount": {"type": "integer", "minimum": 0, "description": "Total shape records before query filtering."},
    "rows":       {"type": "array", "items": {"type": "object"}, "description": "Result rows after filter/project/groupBy/agg."},
    "count":      {"type": "integer", "minimum": 0, "description": "Number of rows returned."},
    "limited":    {"type": "boolean", "description": "True when results were truncated by the limit."}
  }
}`

type queryParams struct {
	Query json.RawMessage `json:"query,omitempty"`
	officetool.SelectorFields
}

// Query returns the powerpoint.query tool definition.
func Query() tools.Tool {
	return tools.Tool{
		Name:         "powerpoint.query",
		Description:  "Run a JSON-shaped query against the presentation's slide+shape catalog.",
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
	args := map[string]any{}
	if len(p.Query) > 0 {
		args["query"] = json.RawMessage(p.Query)
	}
	return runPayloadSum(ctx, env, p.Selector(), "powerpoint.query", args, func(data any) string {
		count := arrayLen(data, "rows")
		return fmt.Sprintf("Query returned %d row(s).", count)
	})
}
