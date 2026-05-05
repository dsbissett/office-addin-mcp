package powerpointtool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const rebuildSlideFromOutlineSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "powerpoint.rebuildSlideFromOutline parameters",
  "description": "Rewrite the title and/or body bullets of an existing slide in one PowerPoint.run.",
  "type": "object",
  "properties": {
    "slideIndex": {"type": "integer", "minimum": 0, "description": "Zero-based slide index."},
    "title":      {"type": "string", "description": "Replacement title text. Omit to leave unchanged."},
    "bullets":    {"type": "array", "items": {"type": "string"}, "description": "Replacement body bullets, one string per line. Omit to leave body unchanged."},` + targetSelectorBase + `},
  "required": ["slideIndex"],
  "additionalProperties": false
}`

type rebuildSlideFromOutlineParams struct {
	SlideIndex int       `json:"slideIndex"`
	Title      *string   `json:"title,omitempty"`
	Bullets    *[]string `json:"bullets,omitempty"`
	officetool.SelectorFields
}

// RebuildSlideFromOutline returns the powerpoint.rebuildSlideFromOutline tool definition.
func RebuildSlideFromOutline() tools.Tool {
	return tools.Tool{
		Name:        "powerpoint.rebuildSlideFromOutline",
		Description: "Rewrite a slide's title and/or body bullets in one PowerPoint.run. Identifies title vs body shape by placeholder name conventions.",
		Schema:      json.RawMessage(rebuildSlideFromOutlineSchema),
		Run:         runRebuildSlideFromOutline,
	}
}

func runRebuildSlideFromOutline(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p rebuildSlideFromOutlineParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	if p.Title == nil && p.Bullets == nil {
		return tools.Fail(tools.CategoryValidation, "nothing_to_set", "rebuildSlideFromOutline requires at least one of: title, bullets", false)
	}
	args := map[string]any{"slideIndex": p.SlideIndex}
	if p.Title != nil {
		args["title"] = *p.Title
	}
	if p.Bullets != nil {
		args["bullets"] = *p.Bullets
	}
	return runPayloadSum(ctx, env, p.Selector(), "powerpoint.rebuildSlideFromOutline", args, func(data any) string {
		idx := p.SlideIndex
		bullets := 0
		if n, ok := numberField(data, "bulletsSet"); ok {
			bullets = int(n)
		}
		return fmt.Sprintf("Rebuilt slide %d (title %v, %d bullet(s)).", idx, p.Title != nil, bullets)
	})
}
