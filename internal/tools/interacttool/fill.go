package interacttool

import (
	"context"
	"encoding/json"
	"strconv"

	cdpproto "github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const fillSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.fill parameters",
  "type": "object",
  "properties": {
    "uid":        {"type": "string", "minLength": 1},
    "text":       {"type": "string", "description": "Replacement text. Existing value is cleared first."},
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]}
  },
  "required": ["uid", "text"],
  "additionalProperties": false
}`

type fillParams struct {
	UID  string `json:"uid"`
	Text string `json:"text"`
	selectorCommon
}

// Fill returns the page.fill tool. Focuses the input by backendNodeId, clears
// it, and inserts the new text. For <select> elements the value is set
// directly via Runtime.callFunctionOn instead of typing.
func Fill() tools.Tool {
	return tools.Tool{
		Name:        "page.fill",
		Description: "Replace the value of an input/select referenced by snapshot UID with the given text.",
		Schema:      json.RawMessage(fillSchema),
		Annotations: &tools.Annotations{DestructiveHint: tools.BoolPtr(true), IdempotentHint: true},
		Run:         runFill,
	}
}

func runFill(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p fillParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	att, node, objectID, res := resolveFillTarget(ctx, env, p)
	if res.Err != nil {
		return res
	}

	// Detect whether this is a SELECT — if so, set value directly and dispatch
	// change/input. Otherwise, focus + clear + insertText, which fires the
	// usual input/change events on text inputs and contenteditable.
	isSelect, res := isSelectElement(ctx, att, objectID)
	if res.Err != nil {
		return res
	}
	if isSelect {
		return fillSelect(ctx, att, objectID, p)
	}
	return fillInput(ctx, att, node.BackendNodeID, p)
}

// resolveFillTarget attaches to the target, looks up the snapshot node for the
// requested UID, enables the DOM/Runtime domains, and resolves the node's
// Runtime objectId. On success the final return is the zero Result.
func resolveFillTarget(ctx context.Context, env *tools.RunEnv, p fillParams) (*tools.AttachedTarget, *session.SnapshotNode, string, tools.Result) {
	att, err := env.Attach(ctx, p.selector())
	if err != nil {
		return nil, nil, "", tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	node, lookupRes := lookupNode(env, att, p.UID)
	if lookupRes.Err != nil {
		return nil, nil, "", lookupRes
	}
	if res := enableFillDomains(ctx, env, att); res.Err != nil {
		return nil, nil, "", res
	}
	objectID, res := resolveObjectID(ctx, att, node.BackendNodeID)
	if res.Err != nil {
		return nil, nil, "", res
	}
	return att, node, objectID, tools.Result{}
}

// enableFillDomains ensures the DOM and Runtime domains are enabled for the
// session before fill operations that depend on them.
func enableFillDomains(ctx context.Context, env *tools.RunEnv, att *tools.AttachedTarget) tools.Result {
	if err := env.EnsureEnabled(ctx, att.SessionID, "DOM"); err != nil {
		return tools.ClassifyCDPErr("enable_dom_failed", err)
	}
	if err := env.EnsureEnabled(ctx, att.SessionID, "Runtime"); err != nil {
		return tools.ClassifyCDPErr("enable_runtime_failed", err)
	}
	return tools.Result{}
}

// resolveObjectID resolves a backendNodeId to a Runtime objectId via
// DOM.resolveNode. On success the second return is the zero Result.
func resolveObjectID(ctx context.Context, att *tools.AttachedTarget, backendNodeID int) (string, tools.Result) {
	rawObj, err := att.Conn.Send(ctx, att.SessionID, "DOM.resolveNode", map[string]any{
		"backendNodeId": backendNodeID,
	})
	if err != nil {
		return "", tools.ClassifyCDPErr("resolve_node_failed", err)
	}
	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(rawObj, &resolved); err != nil {
		return "", tools.Fail(tools.CategoryProtocol, "resolve_decode", err.Error(), false)
	}
	if resolved.Object.ObjectID == "" {
		return "", tools.Fail(tools.CategoryProtocol, "resolve_no_object", "DOM.resolveNode returned no objectId", false)
	}
	return resolved.Object.ObjectID, tools.Result{}
}

// isSelectElement reports whether the resolved object is a <select>. A failed
// tagName probe surfaces as a tagname_failed error (the zero Result otherwise).
// A tagName JSON decode error is ignored, matching the original best-effort
// behavior of treating an unparsed result as not-a-select.
func isSelectElement(ctx context.Context, att *tools.AttachedTarget, objectID string) (bool, tools.Result) {
	tagRaw, err := att.Conn.Send(ctx, att.SessionID, "Runtime.callFunctionOn", map[string]any{
		"objectId":            objectID,
		"functionDeclaration": "function(){return (this.tagName||'').toLowerCase();}",
		"returnByValue":       true,
	})
	if err != nil {
		return false, tools.ClassifyCDPErr("tagname_failed", err)
	}
	var tagOut struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	_ = json.Unmarshal(tagRaw, &tagOut)
	return tagOut.Result.Value == "select", tools.Result{}
}

// fillSelect sets a <select> value directly and dispatches input/change.
func fillSelect(ctx context.Context, att *tools.AttachedTarget, objectID string, p fillParams) tools.Result {
	if _, err := att.Conn.Send(ctx, att.SessionID, "Runtime.callFunctionOn", map[string]any{
		"objectId":            objectID,
		"functionDeclaration": "function(v){this.value=v;this.dispatchEvent(new Event('input',{bubbles:true}));this.dispatchEvent(new Event('change',{bubbles:true}));return this.value;}",
		"arguments":           []any{map[string]any{"value": p.Text}},
		"returnByValue":       true,
	}); err != nil {
		return tools.ClassifyCDPErr("select_set_failed", err)
	}
	return tools.OKWithSummary(
		"Set <select> "+p.UID+" to "+p.Text+".",
		struct {
			UID  string `json:"uid"`
			Text string `json:"text"`
			Mode string `json:"mode"`
		}{UID: p.UID, Text: p.Text, Mode: "select"},
	)
}

// fillInput focuses the element, clears its value, and inserts new text. Works
// for <input>/<textarea> and contenteditable nodes.
func fillInput(ctx context.Context, att *tools.AttachedTarget, backendNodeID int, p fillParams) tools.Result {
	// Focus the element so subsequent Input.insertText goes to it.
	if _, err := att.Conn.Send(ctx, att.SessionID, "DOM.focus", map[string]any{
		"backendNodeId": backendNodeID,
	}); err != nil {
		return tools.ClassifyCDPErr("focus_failed", err)
	}
	// Clear existing value via JS — works for <input>/<textarea>; for
	// contenteditable nodes set textContent.
	if _, err := att.Conn.Evaluate(ctx, att.SessionID, cdpproto.EvaluateParams{
		Expression:    `(function(){var el=document.activeElement;if(!el)return;if('value' in el){el.value='';el.dispatchEvent(new Event('input',{bubbles:true}));}else{el.textContent='';}})()`,
		ReturnByValue: true,
	}); err != nil {
		return tools.ClassifyCDPErr("clear_failed", err)
	}
	if _, err := att.Conn.Send(ctx, att.SessionID, "Input.insertText", map[string]any{
		"text": p.Text,
	}); err != nil {
		return tools.ClassifyCDPErr("insert_text_failed", err)
	}
	return tools.OKWithSummary(
		"Filled "+p.UID+" with "+strconv.Itoa(len(p.Text))+" character(s).",
		struct {
			UID  string `json:"uid"`
			Text string `json:"text"`
			Mode string `json:"mode"`
		}{UID: p.UID, Text: p.Text, Mode: "input"},
	)
}
