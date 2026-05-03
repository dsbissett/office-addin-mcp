package onenotetool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

// ---------- onenote.readNotebooks ----------

const noSelectorOnlySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "OneNote tool parameters",
  "type": "object",
  "properties": {` + targetSelectorBase + `},
  "additionalProperties": false
}`

// ReadNotebooks returns the onenote.readNotebooks tool definition.
func ReadNotebooks() tools.Tool {
	return tools.Tool{
		Name:        "onenote.readNotebooks",
		Description: "List every notebook visible to the OneNote application: id and name.",
		Schema:      json.RawMessage(noSelectorOnlySchema),
		Run:         runReadNotebooks,
	}
}

func runReadNotebooks(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "onenote.readNotebooks", map[string]any{}, func(data any) string {
		return fmt.Sprintf("Listed %d notebook(s).", arrayLen(data, "notebooks"))
	})
}

// ---------- onenote.readSections ----------

// ReadSections returns the onenote.readSections tool definition.
func ReadSections() tools.Tool {
	return tools.Tool{
		Name:        "onenote.readSections",
		Description: "List sections in the active notebook: id and name.",
		Schema:      json.RawMessage(noSelectorOnlySchema),
		Run:         runReadSections,
	}
}

func runReadSections(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "onenote.readSections", map[string]any{}, func(data any) string {
		notebook := stringField(data, "notebookName")
		n := arrayLen(data, "sections")
		if notebook != "" {
			return fmt.Sprintf("Listed %d section(s) in %s.", n, notebook)
		}
		return fmt.Sprintf("Listed %d section(s).", n)
	})
}

// ---------- onenote.readPages ----------

// ReadPages returns the onenote.readPages tool definition.
func ReadPages() tools.Tool {
	return tools.Tool{
		Name:        "onenote.readPages",
		Description: "List pages in the active section: id and title.",
		Schema:      json.RawMessage(noSelectorOnlySchema),
		Run:         runReadPages,
	}
}

func runReadPages(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "onenote.readPages", map[string]any{}, func(data any) string {
		section := stringField(data, "sectionName")
		n := arrayLen(data, "pages")
		if section != "" {
			return fmt.Sprintf("Listed %d page(s) in %s.", n, section)
		}
		return fmt.Sprintf("Listed %d page(s).", n)
	})
}

// ---------- onenote.readPage ----------

// ReadPage returns the onenote.readPage tool definition.
func ReadPage() tools.Tool {
	return tools.Tool{
		Name:        "onenote.readPage",
		Description: "Read the active page: title and content list (id + type per content item).",
		Schema:      json.RawMessage(noSelectorOnlySchema),
		Run:         runReadPage,
	}
}

func runReadPage(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "onenote.readPage", map[string]any{}, func(data any) string {
		title := stringField(data, "title")
		n := arrayLen(data, "contents")
		if title != "" {
			return fmt.Sprintf("Read page %q (%d content item(s)).", title, n)
		}
		return fmt.Sprintf("Read page (%d content item(s)).", n)
	})
}

// ---------- onenote.addPage ----------

const addPageSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "onenote.addPage parameters",
  "type": "object",
  "properties": {
    "title": {"type": "string", "description": "Title for the new page."},` + targetSelectorBase + `},
  "required": ["title"],
  "additionalProperties": false
}`

type addPageParams struct {
	Title string `json:"title"`
	officetool.SelectorFields
}

// AddPage returns the onenote.addPage tool definition.
func AddPage() tools.Tool {
	return tools.Tool{
		Name:        "onenote.addPage",
		Description: "Append a new page to the active section with the given title.",
		Schema:      json.RawMessage(addPageSchema),
		Run:         runAddPage,
	}
}

func runAddPage(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p addPageParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "onenote.addPage", map[string]any{"title": p.Title}, func(_ any) string {
		return "Added page: " + p.Title
	})
}
