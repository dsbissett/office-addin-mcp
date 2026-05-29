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
		// Destructive: closes (removes) a page target. Idempotent — once the
		// target is closed the end state is the same on repeat.
		Annotations: &tools.Annotations{IdempotentHint: true, DestructiveHint: tools.BoolPtr(true)},
	}
}

func runClose(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p closeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	if res, ok := requireSelector(p.TargetID, p.URLPattern, p.Surface); !ok {
		return res
	}
	targetID, success, errRes, ok := resolveAndClose(ctx, p, env)
	if !ok {
		return errRes
	}
	if env.ClearDefaultSelection != nil {
		env.ClearDefaultSelection()
	}
	return closeResult(targetID, success)
}

// resolveAndClose opens a connection, resolves the selector via the attach
// path (so the wrong target is never closed), and drives Target.closeTarget.
// On any failure it returns ok=false with the error envelope to surface.
func resolveAndClose(ctx context.Context, p closeParams, env *tools.RunEnv) (targetID string, success bool, errRes tools.Result, ok bool) {
	conn, err := env.Conn(ctx)
	if err != nil {
		return "", false, tools.Fail(tools.CategoryConnection, "open_failed", err.Error(), true), false
	}
	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return "", false, tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false), false
	}
	rawRes, err := conn.Send(ctx, "", "Target.closeTarget", map[string]any{
		"targetId": att.Target.TargetID,
	})
	if err != nil {
		return "", false, tools.ClassifyCDPErr("close_failed", err), false
	}
	var out struct {
		Success bool `json:"success"`
	}
	_ = json.Unmarshal(rawRes, &out)
	return att.Target.TargetID, out.Success, tools.Result{}, true
}

// closeResult shapes the OK envelope for a completed Target.closeTarget,
// branching the summary on whether CDP reported success.
func closeResult(targetID string, success bool) tools.Result {
	summary := "Closed page " + targetID + "."
	if !success {
		summary = "Close requested for " + targetID + " but CDP reported success=false."
	}
	return tools.OKWithSummary(
		summary,
		struct {
			TargetID string `json:"targetId"`
			Success  bool   `json:"success"`
		}{TargetID: targetID, Success: success},
	)
}
