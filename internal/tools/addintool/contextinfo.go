package addintool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const contextInfoSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.contextInfo parameters",
  "type": "object",
  "properties": {
    "targetId":   {"type": "string", "description": "Exact target id; mutually exclusive with urlPattern/surface."},
    "urlPattern": {"type": "string", "description": "Substring of the target URL."},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"], "description": "Manifest-classified surface to attach to. Falls back to URL heuristics when no manifest is loaded."},
    "requirementSets": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name":       {"type": "string"},
          "minVersion": {"type": "string"}
        },
        "required": ["name"],
        "additionalProperties": false
      },
      "description": "Override the requirement sets probed. Defaults to addin.StandardRequirementSets plus any sets declared in the loaded manifest."
    }
  },
  "additionalProperties": false
}`

type contextInfoParams struct {
	TargetID        string                 `json:"targetId,omitempty"`
	URLPattern      string                 `json:"urlPattern,omitempty"`
	Surface         addin.SurfaceType      `json:"surface,omitempty"`
	RequirementSets []addin.RequirementSet `json:"requirementSets,omitempty"`
}

// ContextInfo returns the addin.contextInfo tool. The tool attaches to the
// requested target and reports Office.context identity (host, platform,
// languages) plus the supported state of every probed requirement set.
func ContextInfo() tools.Tool {
	return tools.Tool{
		Name:        "addin.contextInfo",
		Description: "Report Office.context identity (host, platform, languages, theme, document URL) and probe requirement sets via Office.context.requirements.isSetSupported. Defaults probe addin.StandardRequirementSets plus manifest-declared sets.",
		Schema:      json.RawMessage(contextInfoSchema),
		Run:         runContextInfo,
	}
}

func runContextInfo(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p contextInfoParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	att, err := env.Attach(ctx, tools.TargetSelector{
		TargetID:   p.TargetID,
		URLPattern: p.URLPattern,
		Surface:    p.Surface,
	})
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}

	sets := p.RequirementSets
	if len(sets) == 0 {
		sets = addin.StandardRequirementSets
		if env.Manifest != nil {
			if m := env.Manifest(); m != nil {
				sets = addin.MergeRequirementSets(sets, m.Requirements)
			}
		}
	}

	exec := officejs.New(att.Conn, att.SessionID)
	out, err := exec.Run(ctx, "addin.contextInfo", map[string]any{"requirementSets": sets})
	if err != nil {
		return mapPayloadError(err)
	}
	var data any
	if err := json.Unmarshal(out, &data); err != nil {
		return tools.Fail(tools.CategoryInternal, "decode_payload_result", err.Error(), false)
	}
	return tools.OK(data)
}
