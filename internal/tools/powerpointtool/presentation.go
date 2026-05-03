package powerpointtool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

// ---------- powerpoint.readPresentation ----------

const readPresentationSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "powerpoint.readPresentation parameters",
  "type": "object",
  "properties": {` + targetSelectorBase + `},
  "additionalProperties": false
}`

// ReadPresentation returns the powerpoint.readPresentation tool definition.
func ReadPresentation() tools.Tool {
	return tools.Tool{
		Name:        "powerpoint.readPresentation",
		Description: "Read top-level presentation metadata: title and slide count.",
		Schema:      json.RawMessage(readPresentationSchema),
		Run:         runReadPresentation,
	}
}

func runReadPresentation(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "powerpoint.readPresentation", map[string]any{}, func(data any) string {
		title := stringField(data, "title")
		count, _ := numberField(data, "slideCount")
		if title != "" {
			return fmt.Sprintf("Read presentation %q (%d slide(s)).", title, int(count))
		}
		return fmt.Sprintf("Read presentation (%d slide(s)).", int(count))
	})
}

// ---------- powerpoint.readSlides ----------

// ReadSlides returns the powerpoint.readSlides tool definition.
func ReadSlides() tools.Tool {
	return tools.Tool{
		Name:        "powerpoint.readSlides",
		Description: "List every slide in the presentation with its id and shape names.",
		Schema:      json.RawMessage(readPresentationSchema),
		Run:         runReadSlides,
	}
}

func runReadSlides(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "powerpoint.readSlides", map[string]any{}, func(data any) string {
		return fmt.Sprintf("Listed %d slide(s).", arrayLen(data, "slides"))
	})
}

// ---------- powerpoint.readSlide ----------

const readSlideSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "powerpoint.readSlide parameters",
  "type": "object",
  "properties": {
    "slideIndex": {"type": "integer", "minimum": 0, "description": "0-based index of the slide to read."},` + targetSelectorBase + `},
  "required": ["slideIndex"],
  "additionalProperties": false
}`

type readSlideParams struct {
	SlideIndex int `json:"slideIndex"`
	officetool.SelectorFields
}

// ReadSlide returns the powerpoint.readSlide tool definition.
func ReadSlide() tools.Tool {
	return tools.Tool{
		Name:        "powerpoint.readSlide",
		Description: "Read the shapes on a specific slide: name, type, position, and size.",
		Schema:      json.RawMessage(readSlideSchema),
		Run:         runReadSlide,
	}
}

func runReadSlide(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p readSlideParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "powerpoint.readSlide", map[string]any{"slideIndex": p.SlideIndex}, func(data any) string {
		return fmt.Sprintf("Read slide %d (%d shape(s)).", p.SlideIndex, arrayLen(data, "shapes"))
	})
}

// ---------- powerpoint.addSlide ----------

// AddSlide returns the powerpoint.addSlide tool definition.
func AddSlide() tools.Tool {
	return tools.Tool{
		Name:        "powerpoint.addSlide",
		Description: "Append a new blank slide to the end of the presentation; returns its id.",
		Schema:      json.RawMessage(readPresentationSchema),
		Run:         runAddSlide,
	}
}

func runAddSlide(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "powerpoint.addSlide", map[string]any{}, func(data any) string {
		id := stringField(data, "id")
		if id != "" {
			return "Added slide " + id + "."
		}
		return "Added slide."
	})
}

// ---------- powerpoint.readSelection ----------

// ReadSelection returns the powerpoint.readSelection tool definition.
func ReadSelection() tools.Tool {
	return tools.Tool{
		Name:        "powerpoint.readSelection",
		Description: "Read the ids of the currently selected slides.",
		Schema:      json.RawMessage(readPresentationSchema),
		Run:         runReadSelection,
	}
}

func runReadSelection(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "powerpoint.readSelection", map[string]any{}, func(data any) string {
		n := arrayLen(data, "slides")
		if n == 0 {
			return "No slides selected."
		}
		return fmt.Sprintf("Read %d selected slide(s).", n)
	})
}
