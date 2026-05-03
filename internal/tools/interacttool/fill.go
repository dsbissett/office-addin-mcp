package interacttool

import (
	"context"
	"encoding/json"
	"strconv"

	cdpproto "github.com/dsbissett/office-addin-mcp/internal/cdp"
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
		Run:         runFill,
	}
}

func runFill(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p fillParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	att, err := env.Attach(ctx, p.selector())
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	node, lookupRes := lookupNode(env, att, p.UID)
	if lookupRes.Err != nil {
		return lookupRes
	}

	if err := env.EnsureEnabled(ctx, att.SessionID, "DOM"); err != nil {
		return tools.ClassifyCDPErr("enable_dom_failed", err)
	}
	if err := env.EnsureEnabled(ctx, att.SessionID, "Runtime"); err != nil {
		return tools.ClassifyCDPErr("enable_runtime_failed", err)
	}

	rawObj, err := att.Conn.Send(ctx, att.SessionID, "DOM.resolveNode", map[string]any{
		"backendNodeId": node.BackendNodeID,
	})
	if err != nil {
		return tools.ClassifyCDPErr("resolve_node_failed", err)
	}
	var resolved struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := json.Unmarshal(rawObj, &resolved); err != nil {
		return tools.Fail(tools.CategoryProtocol, "resolve_decode", err.Error(), false)
	}
	objectID := resolved.Object.ObjectID
	if objectID == "" {
		return tools.Fail(tools.CategoryProtocol, "resolve_no_object", "DOM.resolveNode returned no objectId", false)
	}

	// Detect whether this is a SELECT — if so, set value directly and dispatch
	// change/input. Otherwise, focus + clear + insertText, which fires the
	// usual input/change events on text inputs and contenteditable.
	tagRaw, err := att.Conn.Send(ctx, att.SessionID, "Runtime.callFunctionOn", map[string]any{
		"objectId":            objectID,
		"functionDeclaration": "function(){return (this.tagName||'').toLowerCase();}",
		"returnByValue":       true,
	})
	if err != nil {
		return tools.ClassifyCDPErr("tagname_failed", err)
	}
	var tagOut struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	_ = json.Unmarshal(tagRaw, &tagOut)

	if tagOut.Result.Value == "select" {
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

	// Focus the element so subsequent Input.insertText goes to it.
	if _, err := att.Conn.Send(ctx, att.SessionID, "DOM.focus", map[string]any{
		"backendNodeId": node.BackendNodeID,
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
