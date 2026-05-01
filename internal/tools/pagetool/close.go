package pagetool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const closeSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "pages.close parameters",
  "type": "object",
  "description": "Provide exactly one of targetId, urlPattern, or surface.",
  "properties": {
    "targetId":   {"type": "string", "description": "Exact CDP target id."},
    "urlPattern": {"type": "string", "description": "Substring matched against target URL."},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"], "description": "Office add-in surface kind."}
  },
  "additionalProperties": false
}`

type closeParams struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
	Surface    string `json:"surface,omitempty"`
}

// Close returns the pages.close tool. Calls Target.closeTarget on the chosen
// target and clears any sticky default that was pointing at it.
func Close() tools.Tool {
	return tools.Tool{
		Name:        "pages.close",
		Description: "Close a CDP page target. Clears the sticky default if it pointed at the closed target.",
		Schema:      json.RawMessage(closeSchema),
		Run:         runClose,
	}
}

func runClose(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p closeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	if p.TargetID == "" && p.URLPattern == "" && p.Surface == "" {
		return tools.Fail(tools.CategoryValidation, "missing_selector", "provide one of: targetId, urlPattern, surface", false)
	}
	conn, err := env.Conn(ctx)
	if err != nil {
		return tools.Fail(tools.CategoryConnection, "open_failed", err.Error(), true)
	}
	// Resolve via the existing selector path so we never close the wrong target.
	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	rawRes, err := conn.Send(ctx, "", "Target.closeTarget", map[string]any{
		"targetId": att.Target.TargetID,
	})
	if err != nil {
		return tools.ClassifyCDPErr("close_failed", err)
	}
	var out struct {
		Success bool `json:"success"`
	}
	_ = json.Unmarshal(rawRes, &out)

	if env.ClearDefaultSelection != nil {
		env.ClearDefaultSelection()
	}
	return tools.OK(struct {
		TargetID string `json:"targetId"`
		Success  bool   `json:"success"`
	}{TargetID: att.Target.TargetID, Success: out.Success})
}
