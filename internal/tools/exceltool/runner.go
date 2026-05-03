// Package exceltool registers excel.* tools backed by Office.js payloads
// embedded in internal/js. Each tool decodes its params, picks a target via
// the shared selector, attaches, and dispatches the corresponding payload
// through internal/officejs.Executor.
package exceltool

import (
	"context"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

// targetSelectorBase is the shared block of selector fields embedded in every
// excel.* tool's parameter schema.
const targetSelectorBase = `
    "targetId":   {"type": "string", "description": "Exact target id; mutually exclusive with urlPattern."},
    "urlPattern": {"type": "string", "description": "Substring of the target URL (e.g. add-in taskpane URL)."}
`

// maxCells is the Phase 5 read-truncation cap. Range-reading payloads return
// only the top-left cell of any range whose total cell count exceeds this and
// flag truncated=true; mirrors the reference excel-webview2-mcp behavior.
const maxCells = 1000

// runPayloadSum forwards to officetool.RunPayload, tagging error summaries with
// the "Excel" host label so chat clients show the originating host. Excel
// payloads keep this thin wrapper so callers in this package don't have to
// pass the host label at every call site.
func runPayloadSum(ctx context.Context, env *tools.RunEnv, sel tools.TargetSelector, payload string, args any, summaryFn func(any) string) tools.Result {
	return officetool.RunPayload(ctx, env, sel, payload, args, summaryFn, "Excel")
}

// arrayLen returns the length of data[key] when it's a JSON array; otherwise 0.
// Used by list-style summaries that count items in the payload.
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

// boolField returns data[key] when it's a bool; otherwise false.
func boolField(data any, key string) bool {
	m, ok := data.(map[string]any)
	if !ok {
		return false
	}
	b, _ := m[key].(bool)
	return b
}

type selectorFields struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
}

func (s selectorFields) selector() tools.TargetSelector {
	return tools.TargetSelector{TargetID: s.TargetID, URLPattern: s.URLPattern}
}
