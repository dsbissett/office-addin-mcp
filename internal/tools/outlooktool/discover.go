package outlooktool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const discoverSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "outlook.discover parameters",
  "description": "One-call mailbox discovery with persistent caching: user profile + active item context + fingerprint.",
  "type": "object",
  "properties": {
    "force": {"type": "boolean", "description": "Bypass the cache and re-run discovery."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

type discoverParams struct {
	Force bool `json:"force,omitempty"`
	officetool.SelectorFields
}

// Discover returns the outlook.discover tool definition.
func Discover() tools.Tool {
	return tools.Tool{
		Name:        "outlook.discover",
		Description: "Cached Outlook discovery: user profile, host mode, active item identifiers.",
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
	return officetool.RunDiscover(ctx, env, p.Selector(), "outlook", "outlook.discover", p.Force, "Outlook")
}
