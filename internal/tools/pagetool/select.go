package pagetool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const selectSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "pages.select parameters",
  "type": "object",
  "description": "Provide exactly one of targetId, urlPattern, or surface.",
  "properties": {
    "targetId":   {"type": "string", "description": "Exact CDP target id."},
    "urlPattern": {"type": "string", "description": "Substring matched against target URL."},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"], "description": "Office add-in surface kind."}
  },
  "additionalProperties": false
}`

type selectParams struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
	Surface    string `json:"surface,omitempty"`
}

// Select returns the pages.select tool. It resolves the chosen target,
// attaches, and stores a sticky session-level default so subsequent
// UID-based tools (page.click / page.fill / …) operate on that page without
// re-passing the selector.
func Select() tools.Tool {
	return tools.Tool{
		Name:        "pages.select",
		Description: "Pick a sticky default page for subsequent UID-based interaction tools. Resolves and attaches the target so the next call skips Target.getTargets / attachToTarget.",
		Schema:      json.RawMessage(selectSchema),
		Run:         runSelect,
	}
}

func runSelect(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p selectParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	if p.TargetID == "" && p.URLPattern == "" && p.Surface == "" {
		return tools.Fail(tools.CategoryValidation, "missing_selector", "provide one of: targetId, urlPattern, surface", false)
	}
	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	if env.SetDefaultSelection != nil {
		env.SetDefaultSelection(att.Target, att.SessionID)
	}
	label := att.Target.Title
	if label == "" {
		label = att.Target.URL
	}
	return tools.OKWithSummary(
		"Selected page "+label+".",
		struct {
			TargetID     string `json:"targetId"`
			URL          string `json:"url"`
			Title        string `json:"title,omitempty"`
			CDPSessionID string `json:"cdpSessionId"`
		}{
			TargetID:     att.Target.TargetID,
			URL:          att.Target.URL,
			Title:        att.Target.Title,
			CDPSessionID: att.SessionID,
		},
	)
}
