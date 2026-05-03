package wordtool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

// ---------- word.readBody ----------

const readBodySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "word.readBody parameters",
  "type": "object",
  "properties": {` + targetSelectorBase + `},
  "additionalProperties": false
}`

// ReadBody returns the word.readBody tool definition.
func ReadBody() tools.Tool {
	return tools.Tool{
		Name:        "word.readBody",
		Description: "Read the entire document body as plain text.",
		Schema:      json.RawMessage(readBodySchema),
		Run:         runReadBody,
	}
}

func runReadBody(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "word.readBody", map[string]any{}, func(data any) string {
		text := stringField(data, "text")
		if text == "" {
			return "Document body is empty."
		}
		return fmt.Sprintf("Read %d characters from document body.", len(text))
	})
}

// ---------- word.writeBody ----------

const writeBodySchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "word.writeBody parameters",
  "type": "object",
  "properties": {
    "text":     {"type": "string", "description": "Text to insert into the document body."},
    "location": {"type": "string", "enum": ["Replace", "Start", "End"], "description": "Insertion location relative to the body. Defaults to Replace."},` + targetSelectorBase + `},
  "required": ["text"],
  "additionalProperties": false
}`

type writeBodyParams struct {
	Text     string `json:"text"`
	Location string `json:"location,omitempty"`
	officetool.SelectorFields
}

// WriteBody returns the word.writeBody tool definition.
func WriteBody() tools.Tool {
	return tools.Tool{
		Name:        "word.writeBody",
		Description: "Insert text into the document body, replacing or appending depending on location.",
		Schema:      json.RawMessage(writeBodySchema),
		Run:         runWriteBody,
	}
}

func runWriteBody(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p writeBodyParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	location := p.Location
	if location == "" {
		location = "Replace"
	}
	args := map[string]any{"text": p.Text, "location": location}
	return runPayloadSum(ctx, env, p.Selector(), "word.writeBody", args, func(_ any) string {
		return fmt.Sprintf("Wrote %d characters to document body (%s).", len(p.Text), location)
	})
}

// ---------- word.readParagraphs ----------

const readParagraphsSchema = readBodySchema

// ReadParagraphs returns the word.readParagraphs tool definition.
func ReadParagraphs() tools.Tool {
	return tools.Tool{
		Name:        "word.readParagraphs",
		Description: "List all paragraphs in the document body with their text and style.",
		Schema:      json.RawMessage(readParagraphsSchema),
		Run:         runReadParagraphs,
	}
}

func runReadParagraphs(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "word.readParagraphs", map[string]any{}, func(data any) string {
		return fmt.Sprintf("Read %d paragraph(s).", arrayLen(data, "paragraphs"))
	})
}

// ---------- word.insertParagraph ----------

const insertParagraphSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "word.insertParagraph parameters",
  "type": "object",
  "properties": {
    "text":     {"type": "string", "description": "Paragraph text to insert."},
    "location": {"type": "string", "enum": ["Start", "End", "Before", "After"], "description": "Insertion location relative to the body. Defaults to End."},` + targetSelectorBase + `},
  "required": ["text"],
  "additionalProperties": false
}`

type insertParagraphParams struct {
	Text     string `json:"text"`
	Location string `json:"location,omitempty"`
	officetool.SelectorFields
}

// InsertParagraph returns the word.insertParagraph tool definition.
func InsertParagraph() tools.Tool {
	return tools.Tool{
		Name:        "word.insertParagraph",
		Description: "Insert a paragraph into the document body and return its style.",
		Schema:      json.RawMessage(insertParagraphSchema),
		Run:         runInsertParagraph,
	}
}

func runInsertParagraph(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p insertParagraphParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	location := p.Location
	if location == "" {
		location = "End"
	}
	args := map[string]any{"text": p.Text, "location": location}
	return runPayloadSum(ctx, env, p.Selector(), "word.insertParagraph", args, func(data any) string {
		style := stringField(data, "style")
		if style != "" {
			return fmt.Sprintf("Inserted paragraph at %s (style=%s).", location, style)
		}
		return "Inserted paragraph at " + location + "."
	})
}

// ---------- word.readSelection ----------

// ReadSelection returns the word.readSelection tool definition.
func ReadSelection() tools.Tool {
	return tools.Tool{
		Name:        "word.readSelection",
		Description: "Read the text of the current selection in the document.",
		Schema:      json.RawMessage(readBodySchema),
		Run:         runReadSelection,
	}
}

func runReadSelection(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "word.readSelection", map[string]any{}, func(data any) string {
		text := stringField(data, "text")
		if text == "" {
			return "No active selection."
		}
		return fmt.Sprintf("Read selection (%d characters).", len(text))
	})
}

// ---------- word.searchText ----------

const searchTextSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "word.searchText parameters",
  "type": "object",
  "properties": {
    "query":          {"type": "string", "description": "Substring to search for in the document body."},
    "matchCase":      {"type": "boolean", "description": "If true, case-sensitive search."},
    "matchWholeWord": {"type": "boolean", "description": "If true, match whole words only."},` + targetSelectorBase + `},
  "required": ["query"],
  "additionalProperties": false
}`

type searchTextParams struct {
	Query          string `json:"query"`
	MatchCase      *bool  `json:"matchCase,omitempty"`
	MatchWholeWord *bool  `json:"matchWholeWord,omitempty"`
	officetool.SelectorFields
}

// SearchText returns the word.searchText tool definition.
func SearchText() tools.Tool {
	return tools.Tool{
		Name:        "word.searchText",
		Description: "Search the document body for a substring; returns the text of each match.",
		Schema:      json.RawMessage(searchTextSchema),
		Run:         runSearchText,
	}
}

func runSearchText(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p searchTextParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{"query": p.Query}
	if p.MatchCase != nil {
		args["matchCase"] = *p.MatchCase
	}
	if p.MatchWholeWord != nil {
		args["matchWholeWord"] = *p.MatchWholeWord
	}
	return runPayloadSum(ctx, env, p.Selector(), "word.searchText", args, func(data any) string {
		return fmt.Sprintf("Found %d match(es) for %q.", arrayLen(data, "matches"), p.Query)
	})
}

// ---------- word.readProperties ----------

// ReadProperties returns the word.readProperties tool definition.
func ReadProperties() tools.Tool {
	return tools.Tool{
		Name:        "word.readProperties",
		Description: "Read the document's built-in properties (title, author, dates, etc.).",
		Schema:      json.RawMessage(readBodySchema),
		Run:         runReadProperties,
	}
}

func runReadProperties(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p emptySelectorParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return runPayloadSum(ctx, env, p.Selector(), "word.readProperties", map[string]any{}, func(data any) string {
		title := stringField(data, "title")
		if title != "" {
			return "Read properties for " + title + "."
		}
		return "Read document properties."
	})
}
