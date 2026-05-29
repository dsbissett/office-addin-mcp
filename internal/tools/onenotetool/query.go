package onenotetool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const querySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "onenote.query parameters",
  "description": "Run a JSON-shaped filter/project query against pages in the active OneNote section. Records: {id, title}.",
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

// queryOutputSchema describes the success Data shape returned by the
// onenote.query payload: a small filtered answer over pages in the active
// section, with section context and pagination metadata.
const queryOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "onenote.query result",
  "type": "object",
  "properties": {
    "sectionId":   {"type": "string"},
    "sectionName": {"type": "string"},
    "pageCount":   {"type": "integer", "description": "Total pages scanned in the active section before filtering."},
    "rows":        {"type": "array", "items": {"type": "object"}, "description": "Filtered/projected page records."},
    "count":       {"type": "integer", "description": "Number of rows returned."},
    "limited":     {"type": "boolean", "description": "True when the result was capped by query.limit."}
  },
  "required": ["rows", "count"]
}`

type queryParams struct {
	Query json.RawMessage `json:"query,omitempty"`
	officetool.SelectorFields
}

// Query returns the onenote.query tool definition.
func Query() tools.Tool {
	return tools.Tool{
		Name:         "onenote.query",
		Description:  "Run a JSON-shaped query against pages in the active OneNote section.",
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
	return runPayloadSum(ctx, env, p.Selector(), "onenote.query", args, func(data any) string {
		count := arrayLen(data, "rows")
		return fmt.Sprintf("Query returned %d row(s).", count)
	})
}
