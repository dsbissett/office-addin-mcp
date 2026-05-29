package onenotetool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const appendToPageSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "onenote.appendToPage parameters",
  "description": "Append HTML and/or a bullet list to a OneNote page (active by default) in one call.",
  "type": "object",
  "properties": {
    "pageId":  {"type": "string", "description": "Optional page id; defaults to the active page."},
    "html":    {"type": "string", "description": "Raw HTML appended as a new outline. May be combined with bullets."},
    "bullets": {"type": "array", "items": {"type": "string"}, "description": "Bullet items appended as a new outline."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

type appendToPageParams struct {
	PageID  string   `json:"pageId,omitempty"`
	HTML    string   `json:"html,omitempty"`
	Bullets []string `json:"bullets,omitempty"`
	officetool.SelectorFields
}

// AppendToPage returns the onenote.appendToPage tool definition.
func AppendToPage() tools.Tool {
	return tools.Tool{
		Name:        "onenote.appendToPage",
		Description: "Append HTML and/or bullet content to a OneNote page in one call.",
		Schema:      json.RawMessage(appendToPageSchema),
		// Appends new outlines to the page; it never overwrites or deletes
		// existing content, so it is additive (DestructiveHint=false) per the
		// MCP spec's additive-vs-destructive distinction.
		Annotations: &tools.Annotations{DestructiveHint: tools.BoolPtr(false)},
		Run:         runAppendToPage,
	}
}

func runAppendToPage(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p appendToPageParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	if p.HTML == "" && len(p.Bullets) == 0 {
		return tools.Fail(tools.CategoryValidation, "nothing_to_append", "appendToPage requires html or bullets", false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "onenote.appendToPage", appendToPageArgs(p), appendToPageSummary)
}

func appendToPageArgs(p appendToPageParams) map[string]any {
	args := map[string]any{}
	if p.PageID != "" {
		args["pageId"] = p.PageID
	}
	if p.HTML != "" {
		args["html"] = p.HTML
	}
	if len(p.Bullets) > 0 {
		args["bullets"] = p.Bullets
	}
	return args
}

func appendToPageSummary(data any) string {
	title := stringField(data, "title")
	if title == "" {
		title = "page"
	}
	return fmt.Sprintf("Appended content to %q.", title)
}
