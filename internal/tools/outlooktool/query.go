package outlooktool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const querySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "outlook.query parameters",
  "description": "Run a JSON-shaped filter/project query against Outlook items reachable from the active mail context. Folder-wide enumeration is out of scope (no REST token in v1) — the active item is projected and the query runs against that record set.",
  "type": "object",
  "properties": {
    "query": {
      "type": "object",
      "properties": {
        "filter":  {"description": "Filter predicate; same DSL as excel.query.query.filter."},
        "project": {"type": "array", "items": {"type": "string"}},
        "limit":   {"type": "integer", "minimum": 1}
      },
      "additionalProperties": false
    },` + targetSelectorBase + `},
  "additionalProperties": false
}`

const queryOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "outlook.query result",
  "type": "object",
  "required": ["folder", "rows", "count", "limited"],
  "properties": {
    "folder":  {"type": "string", "description": "Source scope; v1 always reports the active item context."},
    "rows":    {"type": "array", "items": {"type": "object"}, "description": "Projected/filtered item records."},
    "count":   {"type": "integer", "description": "Number of rows after filtering."},
    "limited": {"type": "boolean", "description": "True when the result set was truncated by the query limit."},
    "note":    {"type": "string", "description": "Scope caveat about folder-wide enumeration."}
  }
}`

type queryParams struct {
	Query json.RawMessage `json:"query,omitempty"`
	officetool.SelectorFields
}

// Query returns the outlook.query tool definition.
func Query() tools.Tool {
	return tools.Tool{
		Name:         "outlook.query",
		Description:  "Run a JSON-shaped query against Outlook items reachable from the active mail context.",
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
	return runPayloadSum(ctx, env, p.Selector(), "outlook.query", args, func(data any) string {
		count := arrayLen(data, "rows")
		return fmt.Sprintf("Query returned %d row(s).", count)
	})
}
