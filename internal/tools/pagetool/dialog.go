package pagetool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const handleDialogSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "pages.handleDialog parameters",
  "type": "object",
  "properties": {
    "accept":     {"type": "boolean", "description": "true to accept the dialog (OK), false to dismiss (Cancel)."},
    "promptText": {"type": "string", "description": "Text to fill into a prompt() dialog. Ignored for alert/confirm."},
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]}
  },
  "required": ["accept"],
  "additionalProperties": false
}`

type handleDialogParams struct {
	Accept     bool   `json:"accept"`
	PromptText string `json:"promptText,omitempty"`
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
	Surface    string `json:"surface,omitempty"`
}

// HandleDialog returns the pages.handleDialog tool. Accepts or dismisses a
// pending native browser dialog (alert/confirm/prompt/beforeunload) via
// Page.handleJavaScriptDialog.
func HandleDialog() tools.Tool {
	return tools.Tool{
		Name:        "pages.handleDialog",
		Description: "Accept or dismiss a pending native browser dialog (alert/confirm/prompt/beforeunload).",
		Schema:      json.RawMessage(handleDialogSchema),
		Run:         runHandleDialog,
	}
}

func runHandleDialog(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p handleDialogParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	if err := env.EnsureEnabled(ctx, att.SessionID, "Page"); err != nil {
		return tools.ClassifyCDPErr("enable_page_failed", err)
	}
	args := map[string]any{"accept": p.Accept}
	if p.PromptText != "" {
		args["promptText"] = p.PromptText
	}
	if _, err := att.Conn.Send(ctx, att.SessionID, "Page.handleJavaScriptDialog", args); err != nil {
		return tools.ClassifyCDPErr("handle_dialog_failed", err)
	}
	verb := "Dismissed"
	if p.Accept {
		verb = "Accepted"
	}
	return tools.OKWithSummary(
		verb+" native browser dialog.",
		struct {
			Accepted bool `json:"accepted"`
		}{Accepted: p.Accept},
	)
}
