package exceltool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const applyDiffSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.applyDiff parameters",
  "description": "Apply a batch of cell/range patches in a single Excel.run — one CDP round-trip per call, regardless of patch count.",
  "type": "object",
  "properties": {
    "patches": {
      "type": "array",
      "minItems": 1,
      "description": "Patches applied in order; later patches overwrite earlier ones touching the same cells.",
      "items": {
        "type": "object",
        "properties": {
          "address":      {"type": "string", "minLength": 1, "description": "Range address; required."},
          "sheet":        {"type": "string", "description": "Sheet name; defaults to active sheet (or sheet embedded in address)."},
          "value":        {"description": "Scalar value applied to a single cell. Mutually exclusive with values."},
          "values":       {"type": "array", "description": "2-D array of values matching the range shape."},
          "formula":      {"type": "string", "description": "Scalar formula applied to a single cell."},
          "formulas":     {"type": "array", "description": "2-D array of formulas matching the range shape."},
          "numberFormat": {"description": "Either a string applied uniformly or a 2-D array matching the range shape."}
        },
        "required": ["address"],
        "additionalProperties": false
      }
    },` + targetSelectorBase + `},
  "required": ["patches"],
  "additionalProperties": false
}`

type applyDiffParams struct {
	Patches []json.RawMessage `json:"patches"`
	selectorFields
}

// ApplyDiff returns the excel.applyDiff tool definition.
func ApplyDiff() tools.Tool {
	return tools.Tool{
		Name:        "excel.applyDiff",
		Description: "Apply a batch of cell/range patches (values, formulas, number formats) in one Excel.run. One CDP round-trip per call.",
		Schema:      json.RawMessage(applyDiffSchema),
		Run:         runApplyDiff,
	}
}

func runApplyDiff(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p applyDiffParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	if len(p.Patches) == 0 {
		return tools.Fail(tools.CategoryValidation, "no_patches", "patches must contain at least one entry", false)
	}
	args := map[string]any{"patches": p.Patches}
	return runPayloadSum(ctx, env, p.selector(), "excel.applyDiff", args, func(data any) string {
		count := arrayLen(data, "applied")
		return fmt.Sprintf("Applied %d patch(es).", count)
	})
}
