// Package wordtool registers word.* tools backed by Office.js payloads
// embedded in internal/js. Each tool decodes its params, picks a target via
// the shared selector, attaches, and dispatches the corresponding payload
// through internal/officejs.Executor — the dispatch loop itself lives in
// internal/tools/officetool.
package wordtool

import (
	"context"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

// targetSelectorBase is the shared block of selector fields embedded in every
// word.* tool's parameter schema. Sourced from officetool so all host packages
// stay byte-identical here.
const targetSelectorBase = officetool.TargetSelectorBase

// runPayloadSum forwards to officetool.RunPayload, tagging error summaries with
// the "Word" host label so chat clients show the originating host.
func runPayloadSum(ctx context.Context, env *tools.RunEnv, sel tools.TargetSelector, payload string, args any, summaryFn func(any) string) tools.Result {
	return officetool.RunPayload(ctx, env, sel, payload, args, summaryFn, "Word")
}

// emptySelectorParams is for tools that take only the selector fields.
type emptySelectorParams struct {
	officetool.SelectorFields
}

// arrayLen returns the length of data[key] when it's a JSON array; otherwise 0.
func arrayLen(data any, key string) int {
	m, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	arr, ok := m[key].([]any)
	if !ok {
		return 0
	}
	return len(arr)
}

// stringField returns data[key] when it's a string; otherwise "".
func stringField(data any, key string) string {
	m, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m[key].(string)
	return s
}
