package addintool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const cfRuntimeInfoSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.cfRuntimeInfo parameters",
  "type": "object",
  "properties": {
    "targetId":   {"type": "string", "description": "Target id of the custom-functions runtime. Defaults to the cf-runtime surface."},
    "urlPattern": {"type": "string", "description": "URL substring of the custom-functions runtime."}
  },
  "additionalProperties": false
}`

type cfRuntimeParams struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
}

// CFRuntimeInfo returns the addin.cfRuntimeInfo tool. Best-effort probe of
// the custom-functions runtime's registered association map. Falls back to
// available=false when CustomFunctions is not exposed in the chosen target.
func CFRuntimeInfo() tools.Tool {
	return tools.Tool{
		Name:        "addin.cfRuntimeInfo",
		Description: "Probe the custom-functions runtime for registered functions. Best-effort: reads CustomFunctions._association.mappings if exposed.",
		Schema:      json.RawMessage(cfRuntimeInfoSchema),
		Run:         runCFRuntimeInfo,
	}
}

func runCFRuntimeInfo(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p cfRuntimeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	sel := tools.TargetSelector{
		TargetID:   p.TargetID,
		URLPattern: p.URLPattern,
	}
	if sel.TargetID == "" && sel.URLPattern == "" {
		sel.Surface = addin.SurfaceCFRuntime
	}
	att, err := env.Attach(ctx, sel)
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	exec := officejs.New(att.Conn, att.SessionID)
	out, err := exec.Run(ctx, "addin.cfRuntimeInfo", map[string]any{})
	if err != nil {
		return mapPayloadError(err)
	}
	return decodePayloadResult(out)
}
