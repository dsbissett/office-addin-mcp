package interacttool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const typeTextSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.typeText parameters",
  "type": "object",
  "properties": {
    "text":       {"type": "string", "minLength": 1, "description": "Text to insert at the currently focused element."},
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]}
  },
  "required": ["text"],
  "additionalProperties": false
}`

type typeTextParams struct {
	Text string `json:"text"`
	selectorCommon
}

// TypeText returns the page.typeText tool. Sends Input.insertText to whatever
// currently has focus on the active page. Useful when the agent has already
// driven focus (via page.click or page.fill) and just needs to append.
func TypeText() tools.Tool {
	return tools.Tool{
		Name:        "page.typeText",
		Description: "Insert text at the currently focused element via Input.insertText. Use after page.click/page.fill has set focus.",
		Schema:      json.RawMessage(typeTextSchema),
		Run:         runTypeText,
	}
}

func runTypeText(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p typeTextParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	att, err := env.Attach(ctx, p.selector())
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	if _, err := att.Conn.Send(ctx, att.SessionID, "Input.insertText", map[string]any{
		"text": p.Text,
	}); err != nil {
		return tools.ClassifyCDPErr("insert_text_failed", err)
	}
	return tools.OKWithSummary(
		fmt.Sprintf("Typed %d character(s) at focused element.", len(p.Text)),
		struct {
			Text string `json:"text"`
		}{Text: p.Text},
	)
}
