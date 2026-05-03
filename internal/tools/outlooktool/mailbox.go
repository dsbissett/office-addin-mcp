package outlooktool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

// ---------- outlook.readItem ----------

const readItemSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "outlook.readItem parameters",
  "type": "object",
  "properties": {` + targetSelectorBase + `},
  "additionalProperties": false
}`

// ReadItem returns the outlook.readItem tool definition.
func ReadItem() tools.Tool {
	return tools.Tool{
		Name:        "outlook.readItem",
		Description: "Read core properties of the currently selected mailbox item: subject, itemType, itemClass, conversationId, dates, itemId.",
		Schema:      json.RawMessage(readItemSchema),
		Run:         runReadItem,
	}
}

func runReadItem(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "outlook.readItem", map[string]any{}, func(data any) string {
		subj := stringField(data, "subject")
		if subj != "" {
			return "Read item: " + subj
		}
		return "Read mailbox item."
	})
}

// ---------- outlook.getBody ----------

const bodySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "outlook.getBody parameters",
  "type": "object",
  "properties": {
    "coercionType": {"type": "string", "enum": ["text", "html"], "description": "Coercion type for the body content. Defaults to text."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

type bodyGetParams struct {
	CoercionType string `json:"coercionType,omitempty"`
	officetool.SelectorFields
}

// GetBody returns the outlook.getBody tool definition.
func GetBody() tools.Tool {
	return tools.Tool{
		Name:        "outlook.getBody",
		Description: "Read the body of the currently selected mailbox item via item.body.getAsync. Defaults to text coercion.",
		Schema:      json.RawMessage(bodySchema),
		Run:         runGetBody,
	}
}

func runGetBody(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p bodyGetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{}
	if p.CoercionType != "" {
		args["coercionType"] = p.CoercionType
	}
	return runPayloadSum(ctx, env, p.Selector(), "outlook.getBody", args, func(data any) string {
		body := stringField(data, "body")
		ct := stringField(data, "coercionType")
		return fmt.Sprintf("Read item body (%d chars, %s).", len(body), ct)
	})
}

// ---------- outlook.setBody ----------

const setBodySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "outlook.setBody parameters",
  "type": "object",
  "properties": {
    "content":      {"type": "string", "description": "Content to set as the item body."},
    "coercionType": {"type": "string", "enum": ["text", "html"], "description": "Coercion type. Defaults to text."},` + targetSelectorBase + `},
  "required": ["content"],
  "additionalProperties": false
}`

type setBodyParams struct {
	Content      string `json:"content"`
	CoercionType string `json:"coercionType,omitempty"`
	officetool.SelectorFields
}

// SetBody returns the outlook.setBody tool definition.
func SetBody() tools.Tool {
	return tools.Tool{
		Name:        "outlook.setBody",
		Description: "Set the body of the currently composed mailbox item via item.body.setAsync. Compose mode only.",
		Schema:      json.RawMessage(setBodySchema),
		Run:         runSetBody,
	}
}

func runSetBody(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p setBodyParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{"content": p.Content}
	if p.CoercionType != "" {
		args["coercionType"] = p.CoercionType
	}
	return runPayloadSum(ctx, env, p.Selector(), "outlook.setBody", args, func(_ any) string {
		return fmt.Sprintf("Set item body (%d chars).", len(p.Content))
	})
}

// ---------- outlook.getSubject ----------

// GetSubject returns the outlook.getSubject tool definition.
func GetSubject() tools.Tool {
	return tools.Tool{
		Name:        "outlook.getSubject",
		Description: "Read the subject of the currently selected mailbox item. Works in both read and compose modes.",
		Schema:      json.RawMessage(readItemSchema),
		Run:         runGetSubject,
	}
}

func runGetSubject(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "outlook.getSubject", map[string]any{}, func(data any) string {
		subj := stringField(data, "subject")
		mode := stringField(data, "mode")
		if subj == "" {
			return "Read subject (empty, mode=" + mode + ")."
		}
		return "Read subject (mode=" + mode + "): " + subj
	})
}

// ---------- outlook.setSubject ----------

const setSubjectSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "outlook.setSubject parameters",
  "type": "object",
  "properties": {
    "subject": {"type": "string", "description": "New subject text."},` + targetSelectorBase + `},
  "required": ["subject"],
  "additionalProperties": false
}`

type setSubjectParams struct {
	Subject string `json:"subject"`
	officetool.SelectorFields
}

// SetSubject returns the outlook.setSubject tool definition.
func SetSubject() tools.Tool {
	return tools.Tool{
		Name:        "outlook.setSubject",
		Description: "Set the subject of the currently composed mailbox item via item.subject.setAsync. Compose mode only.",
		Schema:      json.RawMessage(setSubjectSchema),
		Run:         runSetSubject,
	}
}

func runSetSubject(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p setSubjectParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "outlook.setSubject", map[string]any{"subject": p.Subject}, func(_ any) string {
		return "Set subject to: " + p.Subject
	})
}

// ---------- outlook.getRecipients ----------

// GetRecipients returns the outlook.getRecipients tool definition.
func GetRecipients() tools.Tool {
	return tools.Tool{
		Name:        "outlook.getRecipients",
		Description: "Read To and Cc recipients on the currently selected mailbox item. Works in both read and compose modes.",
		Schema:      json.RawMessage(readItemSchema),
		Run:         runGetRecipients,
	}
}

func runGetRecipients(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "outlook.getRecipients", map[string]any{}, func(data any) string {
		return fmt.Sprintf("Read %d To and %d Cc recipient(s).", arrayLen(data, "to"), arrayLen(data, "cc"))
	})
}
