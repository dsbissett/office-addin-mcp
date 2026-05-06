package powerpointtool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const discoverSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "powerpoint.discover parameters",
  "description": "One-call presentation discovery with persistent caching: title, slide count, per-slide shape counts.",
  "type": "object",
  "properties": {
    "force": {"type": "boolean", "description": "Bypass the cache and re-run discovery."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

type discoverParams struct {
	Force bool `json:"force,omitempty"`
	officetool.SelectorFields
}

// Discover returns the powerpoint.discover tool definition.
func Discover() tools.Tool {
	return tools.Tool{
		Name:        "powerpoint.discover",
		Description: "Cached PowerPoint discovery: title, slide count, shape count per slide.",
		Schema:      json.RawMessage(discoverSchema),
		Annotations: &tools.Annotations{ReadOnlyHint: true},
		Run:         runDiscover,
	}
}

func runDiscover(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p discoverParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return officetool.RunDiscover(ctx, env, p.Selector(), "powerpoint", "powerpoint.discover", p.Force, "PowerPoint")
}
