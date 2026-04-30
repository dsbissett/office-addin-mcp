// Package browsertool registers browser-level tools (browser.navigate) on the
// shared tools.Registry.
package browsertool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const navigateSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "browser.navigate parameters",
  "type": "object",
  "properties": {
    "url":        {"type": "string", "minLength": 1, "description": "URL to navigate to."},
    "targetId":   {"type": "string", "description": "Exact target id; mutually exclusive with urlPattern."},
    "urlPattern": {"type": "string", "description": "Substring of an existing target URL to choose which target to navigate."}
  },
  "required": ["url"],
  "additionalProperties": false
}`

type navigateParams struct {
	URL        string `json:"url"`
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
}

// Navigate returns the browser.navigate tool definition.
func Navigate() tools.Tool {
	return tools.Tool{
		Name:        "browser.navigate",
		Description: "Navigate a CDP target to a URL via Page.navigate. Returns frameId/loaderId or surfaces errorText as a protocol failure.",
		Schema:      json.RawMessage(navigateSchema),
		Run:         runNavigate,
	}
}

func runNavigate(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p navigateParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}

	conn, err := env.OpenConn(ctx)
	if err != nil {
		return tools.Fail(tools.CategoryConnection, "open_failed", err.Error(), true)
	}
	defer conn.Close()

	target, err := tools.ResolveTarget(ctx, conn, tools.TargetSelector{
		TargetID:   p.TargetID,
		URLPattern: p.URLPattern,
	})
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "resolve_target_failed", err.Error(), false)
	}
	env.Diag.TargetID = target.TargetID

	sessionID, err := conn.AttachToTarget(ctx, target.TargetID)
	if err != nil {
		return tools.ClassifyCDPErr("attach_failed", err)
	}
	env.Diag.SessionID = sessionID

	res, err := conn.PageNavigate(ctx, sessionID, p.URL)
	if err != nil {
		return tools.ClassifyCDPErr("navigate_failed", err)
	}
	if res.ErrorText != "" {
		return tools.Fail(tools.CategoryProtocol, "navigate_error", res.ErrorText, false)
	}
	return tools.OK(struct {
		FrameID  string `json:"frameId"`
		LoaderID string `json:"loaderId,omitempty"`
		URL      string `json:"url"`
	}{FrameID: res.FrameID, LoaderID: res.LoaderID, URL: p.URL})
}
