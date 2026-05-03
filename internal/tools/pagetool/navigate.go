package pagetool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const navigateSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.navigate parameters",
  "type": "object",
  "properties": {
    "url":        {"type": "string", "minLength": 1, "description": "URL to navigate to."},
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]}
  },
  "required": ["url"],
  "additionalProperties": false
}`

type navigateParams struct {
	URL        string `json:"url"`
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
	Surface    string `json:"surface,omitempty"`
}

// Navigate returns the page.navigate tool — Phase 4 replacement for
// browser.navigate. Selects a target via the surface-aware selector and
// drives Page.navigate.
func Navigate() tools.Tool {
	return tools.Tool{
		Name:        "page.navigate",
		Description: "Navigate the chosen page target to a URL via Page.navigate. Returns frameId/loaderId or surfaces errorText as a protocol failure.",
		Schema:      json.RawMessage(navigateSchema),
		Run:         runNavigate,
	}
}

func runNavigate(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p navigateParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	res, err := att.Conn.PageNavigate(ctx, att.SessionID, p.URL)
	if err != nil {
		return tools.ClassifyCDPErr("navigate_failed", err)
	}
	if res.ErrorText != "" {
		return tools.Result{
			Err:     &tools.EnvelopeError{Code: "navigate_error", Message: res.ErrorText, Category: tools.CategoryProtocol},
			Summary: "Navigation to " + p.URL + " failed: " + res.ErrorText,
		}
	}
	return tools.OKWithSummary(
		"Navigated to "+p.URL+".",
		struct {
			FrameID  string `json:"frameId"`
			LoaderID string `json:"loaderId,omitempty"`
			URL      string `json:"url"`
		}{FrameID: res.FrameID, LoaderID: res.LoaderID, URL: p.URL},
	)
}
