package exceltool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const summarizeWorkbookSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.summarizeWorkbook parameters",
  "description": "One-call discovery: sheet list, table catalog, named ranges, per-sheet used-range bounds.",
  "type": "object",
  "properties": {` + targetSelectorBase + `},
  "additionalProperties": false
}`

type summarizeWorkbookParams struct {
	selectorFields
}

// SummarizeWorkbook returns the excel.summarizeWorkbook tool definition.
func SummarizeWorkbook() tools.Tool {
	return tools.Tool{
		Name:        "excel.summarizeWorkbook",
		Description: "One-call workbook discovery: sheets, tables, named ranges, per-sheet used-range bounds.",
		Schema:      json.RawMessage(summarizeWorkbookSchema),
		Annotations: &tools.Annotations{ReadOnlyHint: true},
		Run:         runSummarizeWorkbook,
	}
}

func runSummarizeWorkbook(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p summarizeWorkbookParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.summarizeWorkbook", map[string]any{}, func(data any) string {
		sheets := arrayLen(data, "worksheets")
		tables := arrayLen(data, "tables")
		named := arrayLen(data, "namedRanges")
		return fmt.Sprintf("Workbook: %d sheet(s), %d table(s), %d named range(s).", sheets, tables, named)
	})
}
