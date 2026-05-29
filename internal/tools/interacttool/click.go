package interacttool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const clickSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.click parameters",
  "type": "object",
  "properties": {
    "uid":        {"type": "string", "minLength": 1, "description": "Snapshot UID returned by page.snapshot."},
    "button":     {"type": "string", "enum": ["left", "right", "middle"], "description": "Mouse button. Default left."},
    "clickCount": {"type": "integer", "minimum": 1, "maximum": 3, "description": "Number of clicks (e.g. 2 for double-click). Default 1."},
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]}
  },
  "required": ["uid"],
  "additionalProperties": false
}`

type clickParams struct {
	UID        string `json:"uid"`
	Button     string `json:"button,omitempty"`
	ClickCount int    `json:"clickCount,omitempty"`
	selectorCommon
}

// Click returns the page.click tool. Resolves the UID from the snapshot
// cache, fetches the box model, and dispatches mousePressed+mouseReleased at
// the node center.
func Click() tools.Tool {
	return tools.Tool{
		Name:        "page.click",
		Description: "Click a snapshot UID by dispatching mouse press+release at its box-model center.",
		Schema:      json.RawMessage(clickSchema),
		Annotations: &tools.Annotations{DestructiveHint: tools.BoolPtr(true)},
		Run:         runClick,
	}
}

func runClick(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p clickParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	button, clickCount := clickDefaults(p)

	att, err := env.Attach(ctx, p.selector())
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	x, y, node, lookupRes := nodeCenter(ctx, env, att, p.UID)
	if lookupRes.Err != nil {
		return lookupRes
	}

	if res := dispatchClick(ctx, att, x, y, button, clickCount); res.Err != nil {
		return res
	}
	return tools.OKWithSummary(
		fmt.Sprintf("%s %s (%s).", clickVerb(clickCount), nodeLabel(node), p.UID),
		struct {
			UID string  `json:"uid"`
			X   float64 `json:"x"`
			Y   float64 `json:"y"`
		}{UID: p.UID, X: x, Y: y},
	)
}

// clickDefaults resolves the optional button/clickCount params to their
// effective values: button defaults to "left", clickCount to 1.
func clickDefaults(p clickParams) (button string, clickCount int) {
	button = p.Button
	if button == "" {
		button = "left"
	}
	clickCount = p.ClickCount
	if clickCount <= 0 {
		clickCount = 1
	}
	return button, clickCount
}

// dispatchClick sends the mousePressed then mouseReleased events at (x, y). On
// success it returns the zero Result; on CDP failure a classified error.
func dispatchClick(ctx context.Context, att *tools.AttachedTarget, x, y float64, button string, clickCount int) tools.Result {
	common := map[string]any{
		"x":          x,
		"y":          y,
		"button":     button,
		"clickCount": clickCount,
	}
	pressed := mergeMap(common, map[string]any{"type": "mousePressed"})
	released := mergeMap(common, map[string]any{"type": "mouseReleased"})
	if _, err := att.Conn.Send(ctx, att.SessionID, "Input.dispatchMouseEvent", pressed); err != nil {
		return tools.ClassifyCDPErr("mouse_press_failed", err)
	}
	if _, err := att.Conn.Send(ctx, att.SessionID, "Input.dispatchMouseEvent", released); err != nil {
		return tools.ClassifyCDPErr("mouse_release_failed", err)
	}
	return tools.Result{}
}

// nodeLabel renders a snapshot node as `role "name"`, falling back to the bare
// role when the node has no accessible name.
func nodeLabel(node *session.SnapshotNode) string {
	if node.Name != "" {
		return fmt.Sprintf("%s %q", node.Role, node.Name)
	}
	return node.Role
}

// clickVerb maps a click count to its summary verb.
func clickVerb(clickCount int) string {
	switch clickCount {
	case 2:
		return "Double-clicked"
	case 3:
		return "Triple-clicked"
	default:
		return "Clicked"
	}
}

func mergeMap(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
