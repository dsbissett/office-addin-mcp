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

type queryParams struct {
	Query json.RawMessage `json:"query,omitempty"`
	officetool.SelectorFields
}

// Query returns the onenote.query tool definition.
func Query() tools.Tool {
	return tools.Tool{
		Name:        "onenote.query",
		Description: "Run a JSON-shaped query against pages in the active OneNote section.",
		Schema:      json.RawMessage(querySchema),
		Annotations: &tools.Annotations{ReadOnlyHint: true},
		Run:         runQuery,
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
