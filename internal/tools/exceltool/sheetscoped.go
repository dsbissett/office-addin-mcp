package exceltool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const optionalSheetSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "optional-sheet parameters",
  "type": "object",
  "properties": {
    "sheet": {"type": "string", "description": "Worksheet name. Omit to use the active worksheet."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

// ListComments returns the excel.listComments tool definition.
func ListComments() tools.Tool {
	return tools.Tool{
		Name:        "excel.listComments",
		Description: "List comments and replies on a worksheet: author, content, timestamp, and cell address.",
		Schema:      json.RawMessage(optionalSheetSchema),
		Run:         runListComments,
	}
}

func runListComments(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p optionalSheetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	return runPayload(ctx, env, p.selector(), "excel.listComments", args)
}

// ListShapes returns the excel.listShapes tool definition.
func ListShapes() tools.Tool {
	return tools.Tool{
		Name:        "excel.listShapes",
		Description: "List shapes (including images) on a worksheet: name, id, type, position, size, visibility.",
		Schema:      json.RawMessage(optionalSheetSchema),
		Run:         runListShapes,
	}
}

func runListShapes(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p optionalSheetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{}
	if p.Sheet != "" {
		args["sheet"] = p.Sheet
	}
	return runPayload(ctx, env, p.selector(), "excel.listShapes", args)
}
