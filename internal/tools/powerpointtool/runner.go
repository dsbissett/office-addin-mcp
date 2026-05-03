// Package powerpointtool registers powerpoint.* tools backed by Office.js
// payloads embedded in internal/js. PowerPoint payloads dispatch through
// __runPowerPoint, which wraps PowerPoint.run.
package powerpointtool

import (
	"context"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const targetSelectorBase = officetool.TargetSelectorBase

func runPayloadSum(ctx context.Context, env *tools.RunEnv, sel tools.TargetSelector, payload string, args any, summaryFn func(any) string) tools.Result {
	return officetool.RunPayload(ctx, env, sel, payload, args, summaryFn, "PowerPoint")
}

type emptySelectorParams struct {
	officetool.SelectorFields
}

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

func stringField(data any, key string) string {
	m, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func numberField(data any, key string) (float64, bool) {
	m, ok := data.(map[string]any)
	if !ok {
		return 0, false
	}
	n, ok := m[key].(float64)
	return n, ok
}
