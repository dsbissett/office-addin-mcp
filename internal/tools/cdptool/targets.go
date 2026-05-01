// Package cdptool registers raw Chrome DevTools Protocol tools (cdp.*) on the
// shared tools.Registry. These are gated by --expose-raw-cdp at the MCP server
// level.
package cdptool

import (
	"context"
	"encoding/json"

	cdpproto "github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const selectTargetSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "cdp.selectTarget parameters",
  "type": "object",
  "properties": {
    "targetId":   {"type": "string", "description": "Exact CDP target id. Provide this OR urlPattern."},
    "urlPattern": {"type": "string", "description": "Substring matched against target URL. Provide this OR targetId."}
  },
  "additionalProperties": false
}`

type selectTargetParams struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
}

// SelectTarget returns the cdp.selectTarget tool definition.
func SelectTarget() tools.Tool {
	return tools.Tool{
		Name:        "cdp.selectTarget",
		Description: "Resolve a target by id or URL substring and return its TargetInfo. Primes the per-session selector cache so a subsequent CDP call hits without a fresh attach. For high-level page selection prefer pages.select.",
		Schema:      json.RawMessage(selectTargetSchema),
		Run:         runSelectTarget,
	}
}

func runSelectTarget(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p selectTargetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	if p.TargetID == "" && p.URLPattern == "" {
		return tools.Fail(tools.CategoryValidation, "missing_selector", "provide one of: targetId, urlPattern", false)
	}

	att, err := env.Attach(ctx, tools.TargetSelector{TargetID: p.TargetID, URLPattern: p.URLPattern})
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "resolve_target_failed", err.Error(), false)
	}
	return tools.OK(struct {
		Target cdpproto.TargetInfo `json:"target"`
	}{Target: att.Target})
}
