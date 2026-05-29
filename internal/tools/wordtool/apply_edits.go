package wordtool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const applyEditsSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "word.applyEdits parameters",
  "description": "Apply a batch of find/replace edits to the document body in one Word.run.",
  "type": "object",
  "properties": {
    "edits": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "properties": {
          "find":           {"type": "string", "minLength": 1, "description": "Search string passed to Word.Body.search."},
          "replace":        {"type": "string", "description": "Replacement text. Empty deletes the match."},
          "matchCase":      {"type": "boolean", "description": "Case-sensitive match."},
          "matchWholeWord": {"type": "boolean", "description": "Match whole words only."}
        },
        "required": ["find"],
        "additionalProperties": false
      }
    },` + targetSelectorBase + `},
  "required": ["edits"],
  "additionalProperties": false
}`

type applyEditsParams struct {
	Edits []json.RawMessage `json:"edits"`
	officetool.SelectorFields
}

// ApplyEdits returns the word.applyEdits tool definition.
func ApplyEdits() tools.Tool {
	return tools.Tool{
		Name:        "word.applyEdits",
		Description: "Apply a batch of find/replace edits to the Word document body in one Word.run.",
		Schema:      json.RawMessage(applyEditsSchema),
		Annotations: &tools.Annotations{DestructiveHint: tools.BoolPtr(true)},
		Run:         runApplyEdits,
	}
}

func runApplyEdits(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p applyEditsParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	if len(p.Edits) == 0 {
		return tools.Fail(tools.CategoryValidation, "no_edits", "edits must contain at least one entry", false)
	}
	args := map[string]any{"edits": p.Edits}
	return runPayloadSum(ctx, env, p.Selector(), "word.applyEdits", args, func(data any) string {
		total := sumReplacedCounts(data)
		return fmt.Sprintf("Replaced %d occurrence(s) across %d edit(s).", total, len(p.Edits))
	})
}

// sumReplacedCounts totals the "replaced" counts across the edit results in the
// payload data, returning 0 when the shape is not as expected.
func sumReplacedCounts(data any) int {
	m, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	arr, ok := m["edits"].([]any)
	if !ok {
		return 0
	}
	total := 0
	for _, e := range arr {
		total += replacedCount(e)
	}
	return total
}

// replacedCount extracts the "replaced" count from a single edit result,
// returning 0 when the shape is not as expected.
func replacedCount(e any) int {
	ee, ok := e.(map[string]any)
	if !ok {
		return 0
	}
	n, ok := ee["replaced"].(float64)
	if !ok {
		return 0
	}
	return int(n)
}
