package interacttool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const hoverSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.hover parameters",
  "type": "object",
  "properties": {
    "uid":        {"type": "string", "minLength": 1},
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]}
  },
  "required": ["uid"],
  "additionalProperties": false
}`

type hoverParams struct {
	UID string `json:"uid"`
	selectorCommon
}

// Hover returns the page.hover tool. Dispatches mouseMoved at the snapshot
// node's center to trigger hover styles / tooltips.
func Hover() tools.Tool {
	return tools.Tool{
		Name:        "page.hover",
		Description: "Hover the mouse over a snapshot UID by dispatching Input.dispatchMouseEvent type=mouseMoved at its center.",
		Schema:      json.RawMessage(hoverSchema),
		Run:         runHover,
	}
}

func runHover(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p hoverParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	att, err := env.Attach(ctx, p.selector())
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	x, y, _, lookupRes := nodeCenter(ctx, env, att, p.UID)
	if lookupRes.Err != nil {
		return lookupRes
	}
	if _, err := att.Conn.Send(ctx, att.SessionID, "Input.dispatchMouseEvent", map[string]any{
		"type": "mouseMoved",
		"x":    x,
		"y":    y,
	}); err != nil {
		return tools.ClassifyCDPErr("mouse_move_failed", err)
	}
	return tools.OKWithSummary(
		"Hovered "+p.UID+".",
		struct {
			UID string  `json:"uid"`
			X   float64 `json:"x"`
			Y   float64 `json:"y"`
		}{UID: p.UID, X: x, Y: y},
	)
}
