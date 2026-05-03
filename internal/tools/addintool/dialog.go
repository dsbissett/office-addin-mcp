package addintool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const openDialogSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.openDialog parameters",
  "type": "object",
  "properties": {
    "url":              {"type": "string", "minLength": 1, "description": "Absolute https URL of the dialog page. Must be on the same domain as the add-in."},
    "height":           {"type": "number", "description": "Dialog height as a percentage of the screen (1–100)."},
    "width":            {"type": "number", "description": "Dialog width as a percentage of the screen (1–100)."},
    "displayInIframe":  {"type": "boolean", "description": "Render the dialog in an iframe instead of a separate window."},
    "promptBeforeOpen": {"type": "boolean", "description": "Prompt the user before opening when popups are blocked."},
    "targetId":   {"type": "string", "description": "Exact target id to invoke from. Defaults to the taskpane surface."},
    "urlPattern": {"type": "string", "description": "Substring of the target URL to invoke from."},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"], "description": "Manifest-classified surface to invoke from. Defaults to taskpane."}
  },
  "required": ["url"],
  "additionalProperties": false
}`

type openDialogParams struct {
	URL              string            `json:"url"`
	Height           float64           `json:"height,omitempty"`
	Width            float64           `json:"width,omitempty"`
	DisplayInIframe  bool              `json:"displayInIframe,omitempty"`
	PromptBeforeOpen bool              `json:"promptBeforeOpen,omitempty"`
	TargetID         string            `json:"targetId,omitempty"`
	URLPattern       string            `json:"urlPattern,omitempty"`
	Surface          addin.SurfaceType `json:"surface,omitempty"`
}

// OpenDialog returns the addin.openDialog tool. It calls
// Office.context.ui.displayDialogAsync on the chosen target (defaulting to
// the taskpane). The dialog handle is stashed on the page's globalThis so
// addin.dialogClose / addin.dialogSubscribe can reach it without re-opening.
func OpenDialog() tools.Tool {
	return tools.Tool{
		Name:        "addin.openDialog",
		Description: "Open an Office Dialog API dialog from the taskpane (or chosen surface). Persists the dialog handle on the page for subsequent close / message subscription.",
		Schema:      json.RawMessage(openDialogSchema),
		Run:         runOpenDialog,
	}
}

func runOpenDialog(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p openDialogParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	if p.Surface == "" && p.TargetID == "" && p.URLPattern == "" {
		p.Surface = addin.SurfaceTaskpane
	}
	att, err := env.Attach(ctx, tools.TargetSelector{
		TargetID:   p.TargetID,
		URLPattern: p.URLPattern,
		Surface:    p.Surface,
	})
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	args := map[string]any{"url": p.URL}
	if p.Height > 0 {
		args["height"] = p.Height
	}
	if p.Width > 0 {
		args["width"] = p.Width
	}
	if p.DisplayInIframe {
		args["displayInIframe"] = true
	}
	if p.PromptBeforeOpen {
		args["promptBeforeOpen"] = true
	}
	exec := officejs.New(att.Conn, att.SessionID)
	out, err := exec.Run(ctx, "addin.openDialog", args)
	if err != nil {
		return mapPayloadError(err)
	}
	return decodePayloadResultWithSummary(out, "Opened dialog at "+p.URL+".")
}

const dialogCloseSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.dialogClose parameters",
  "type": "object",
  "properties": {
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]}
  },
  "additionalProperties": false
}`

type dialogTargetParams struct {
	TargetID   string            `json:"targetId,omitempty"`
	URLPattern string            `json:"urlPattern,omitempty"`
	Surface    addin.SurfaceType `json:"surface,omitempty"`
}

// DialogClose returns the addin.dialogClose tool. It closes the dialog
// previously opened by addin.openDialog from the same target.
func DialogClose() tools.Tool {
	return tools.Tool{
		Name:        "addin.dialogClose",
		Description: "Close the active Office dialog opened via addin.openDialog from the same target. No-op if no dialog handle is found.",
		Schema:      json.RawMessage(dialogCloseSchema),
		Run:         runDialogPayload("addin.dialogClose"),
	}
}

const dialogSubscribeSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.dialogSubscribe parameters",
  "type": "object",
  "properties": {
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]}
  },
  "additionalProperties": false
}`

// DialogSubscribe returns the addin.dialogSubscribe tool. The first call
// installs DialogMessageReceived/DialogEventReceived handlers; subsequent
// calls drain queued messages.
func DialogSubscribe() tools.Tool {
	return tools.Tool{
		Name:        "addin.dialogSubscribe",
		Description: "Drain Office Dialog API messages and events queued since the previous call. The first invocation installs message/event handlers on the active dialog handle.",
		Schema:      json.RawMessage(dialogSubscribeSchema),
		Run:         runDialogPayload("addin.dialogSubscribe"),
	}
}

func runDialogPayload(toolName string) func(context.Context, json.RawMessage, *tools.RunEnv) tools.Result {
	return func(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
		var p dialogTargetParams
		if err := json.Unmarshal(raw, &p); err != nil {
			return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
		}
		if p.Surface == "" && p.TargetID == "" && p.URLPattern == "" {
			p.Surface = addin.SurfaceTaskpane
		}
		att, err := env.Attach(ctx, tools.TargetSelector{
			TargetID:   p.TargetID,
			URLPattern: p.URLPattern,
			Surface:    p.Surface,
		})
		if err != nil {
			return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
		}
		exec := officejs.New(att.Conn, att.SessionID)
		out, err := exec.Run(ctx, toolName, map[string]any{})
		if err != nil {
			return mapPayloadError(err)
		}
		summary := dialogPayloadSummary(toolName, out)
		return decodePayloadResultWithSummary(out, summary)
	}
}

func dialogPayloadSummary(toolName string, raw json.RawMessage) string {
	switch toolName {
	case "addin.dialogClose":
		var probe struct {
			Closed bool `json:"closed"`
		}
		if err := json.Unmarshal(raw, &probe); err == nil && probe.Closed {
			return "Closed active dialog."
		}
		return "No active dialog handle to close."
	case "addin.dialogSubscribe":
		var probe struct {
			Messages []any `json:"messages"`
			Events   []any `json:"events"`
		}
		if err := json.Unmarshal(raw, &probe); err == nil {
			return fmt.Sprintf("Drained %d message(s) and %d event(s) from dialog.", len(probe.Messages), len(probe.Events))
		}
		return "Drained dialog messages."
	}
	return ""
}

func decodePayloadResultWithSummary(raw json.RawMessage, summary string) tools.Result {
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return tools.Fail(tools.CategoryInternal, "decode_payload_result", err.Error(), false)
	}
	return tools.OKWithSummary(summary, data)
}
