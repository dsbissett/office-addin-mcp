// Package outlooktool registers outlook.* tools backed by Office.js payloads
// embedded in internal/js. Outlook payloads dispatch through __runOutlook
// (preamble), which hands Office.context.mailbox to the body — there is no
// batched-context Outlook.run API.
package outlooktool

import (
	"context"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const targetSelectorBase = officetool.TargetSelectorBase

func runPayloadSum(ctx context.Context, env *tools.RunEnv, sel tools.TargetSelector, payload string, args any, summaryFn func(any) string) tools.Result {
	return officetool.RunPayload(ctx, env, sel, payload, args, summaryFn, "Outlook")
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
