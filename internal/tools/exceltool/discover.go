package exceltool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const discoverSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.discover parameters",
  "description": "One-call workbook discovery with persistent caching. First call within a session populates the cache; subsequent calls return cached data when the workbook fingerprint is unchanged. Pass force=true to bypass.",
  "type": "object",
  "properties": {
    "force": {"type": "boolean", "description": "Bypass the cache and re-run discovery."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

// discoverOutputSchema describes the success Data shape returned by
// excel.discover. The top level merges the excel_discover.js payload
// (filePath, fingerprint, worksheets, tables, namedRanges) with the cache
// metadata (cached) injected by officetool.withCacheMeta; nested item shapes
// are left permissive.
const discoverOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.discover result",
  "type": "object",
  "properties": {
    "cached":      {"type": "boolean"},
    "filePath":    {"type": "string"},
    "fingerprint": {"type": "string"},
    "worksheets":  {"type": "array", "items": {"type": "object"}},
    "tables":      {"type": "array", "items": {"type": "object"}},
    "namedRanges": {"type": "array", "items": {"type": "object"}}
  },
  "required": ["cached", "filePath", "fingerprint", "worksheets", "tables", "namedRanges"]
}`

type discoverParams struct {
	Force bool `json:"force,omitempty"`
	selectorFields
}

// Discover returns the excel.discover tool definition.
func Discover() tools.Tool {
	return tools.Tool{
		Name:         "excel.discover",
		Description:  "Cached workbook discovery: sheets, tables, named ranges, used-range bounds. Cache invalidates automatically when a coarse fingerprint shifts; pass force=true to bypass.",
		Schema:       json.RawMessage(discoverSchema),
		OutputSchema: json.RawMessage(discoverOutputSchema),
		Annotations:  &tools.Annotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: tools.BoolPtr(false)},
		Run:          runDiscover,
	}
}

func runDiscover(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p discoverParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return officetool.RunDiscover(ctx, env, p.selector(), "excel", "excel.discover", p.Force, "Excel")
}
