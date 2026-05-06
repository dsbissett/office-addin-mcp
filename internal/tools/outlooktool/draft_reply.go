package outlooktool

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const draftReplySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "outlook.draftReply parameters",
  "description": "Set subject and/or body on the active compose-mode mailbox item in one call.",
  "type": "object",
  "properties": {
    "subject":      {"type": "string", "description": "Subject to set; omit to leave unchanged."},
    "body":         {"type": "string", "description": "Body content to set; omit to leave unchanged."},
    "coercionType": {"type": "string", "enum": ["html", "text"], "default": "html", "description": "Coercion type for body content."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

type draftReplyParams struct {
	Subject      *string `json:"subject,omitempty"`
	Body         *string `json:"body,omitempty"`
	CoercionType string  `json:"coercionType,omitempty"`
	officetool.SelectorFields
}

// DraftReply returns the outlook.draftReply tool definition.
func DraftReply() tools.Tool {
	return tools.Tool{
		Name:        "outlook.draftReply",
		Description: "Set subject and/or body on a compose-mode Outlook item in one call.",
		Schema:      json.RawMessage(draftReplySchema),
		Run:         runDraftReply,
	}
}

func runDraftReply(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p draftReplyParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	if p.Subject == nil && p.Body == nil {
		return tools.Fail(tools.CategoryValidation, "nothing_to_set", "draftReply requires at least one of: subject, body", false)
	}
	args := map[string]any{}
	if p.Subject != nil {
		args["subject"] = *p.Subject
	}
	if p.Body != nil {
		args["body"] = *p.Body
	}
	if ct := strings.ToLower(p.CoercionType); ct == "html" || ct == "text" {
		args["coercionType"] = ct
	}
	return runPayloadSum(ctx, env, p.Selector(), "outlook.draftReply", args, func(_ any) string {
		var fields []string
		if p.Subject != nil {
			fields = append(fields, "subject")
		}
		if p.Body != nil {
			fields = append(fields, "body")
		}
		return "Drafted reply: set " + strings.Join(fields, " + ") + "."
	})
}
