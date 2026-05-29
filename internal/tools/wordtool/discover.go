package wordtool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const discoverSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "word.discover parameters",
  "description": "One-call document discovery with persistent caching: title, author, sections, content controls, word count.",
  "type": "object",
  "properties": {
    "force": {"type": "boolean", "description": "Bypass the cache and re-run discovery."},` + targetSelectorBase + `},
  "additionalProperties": false
}`

// discoverOutputSchema describes the success Data shape for word.discover. Every
// result merges the cache metadata (cached, filePath, fingerprint) over the live
// or cached discovery snapshot, so those three keys are always present; the
// host-specific fields (title, author, sections, …) ride alongside and are kept
// permissive via additionalProperties.
const discoverOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "word.discover result",
  "type": "object",
  "required": ["cached", "filePath", "fingerprint"],
  "properties": {
    "cached":      {"type": "boolean", "description": "True when the snapshot was served from the persistent doccache."},
    "filePath":    {"type": "string", "description": "Stable file identity of the discovered document."},
    "fingerprint": {"type": "string", "description": "Content fingerprint used for cache invalidation."},
    "title":       {"type": "string"},
    "author":      {"type": "string"},
    "wordCount":   {"type": "number"},
    "sections":    {"type": "array"},
    "contentControls": {"type": "array"}
  },
  "additionalProperties": true
}`

type discoverParams struct {
	Force bool `json:"force,omitempty"`
	officetool.SelectorFields
}

// Discover returns the word.discover tool definition.
func Discover() tools.Tool {
	return tools.Tool{
		Name:         "word.discover",
		Description:  "Cached document discovery: title, author, section count, content controls, word count.",
		Schema:       json.RawMessage(discoverSchema),
		OutputSchema: json.RawMessage(discoverOutputSchema),
		Annotations:  &tools.Annotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: tools.BoolPtr(false)},
		Run:          runDiscover,
	}
}

func runDiscover(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p discoverParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	return officetool.RunDiscover(ctx, env, p.Selector(), "word", "word.discover", p.Force, "Word")
}
