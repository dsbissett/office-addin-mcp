package addintool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const listTargetsSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.listTargets parameters",
  "type": "object",
  "properties": {
    "includeInternal": {"type": "boolean", "description": "Include chrome://, edge://, devtools:// targets in the result. Default false."}
  },
  "additionalProperties": false
}`

type listTargetsParams struct {
	IncludeInternal bool `json:"includeInternal,omitempty"`
}

// ListTargets returns the addin.listTargets tool. It enumerates CDP targets
// and labels each according to the active manifest's surface declarations,
// falling back to URL heuristics when no manifest is loaded.
func ListTargets() tools.Tool {
	return tools.Tool{
		Name:        "addin.listTargets",
		Title:       "List CDP Targets",
		Description: "List CDP targets classified by manifest surface (taskpane / content / dialog / cf-runtime). Falls back to URL heuristics when no add-in manifest is loaded. Use the returned targetId with any tool that accepts a targetId selector.",
		Schema:      json.RawMessage(listTargetsSchema),
		Annotations: &tools.Annotations{
			ReadOnlyHint:    true,
			IdempotentHint:  true,
			DestructiveHint: tools.BoolPtr(false),
		},
		Run: runListTargets,
	}
}

func runListTargets(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p listTargetsParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	conn, err := env.Conn(ctx)
	if err != nil {
		return tools.Fail(tools.CategoryConnection, "open_failed", err.Error(), true)
	}
	targets, err := conn.GetTargets(ctx)
	if err != nil {
		return tools.ClassifyCDPErr("get_targets_failed", err)
	}
	var manifest *addin.Manifest
	if env.Manifest != nil {
		manifest = env.Manifest()
	}
	classified := addin.ClassifyTargets(targets, manifest)

	out := classified[:0]
	for _, c := range classified {
		if !p.IncludeInternal && tools.IsInternalURL(c.URL) {
			continue
		}
		out = append(out, c)
	}
	manifestSuffix := " (no manifest loaded)"
	if manifest != nil {
		manifestSuffix = ""
	}
	return tools.OKWithSummary(
		fmt.Sprintf("Listed %d CDP target(s)%s.", len(out), manifestSuffix),
		struct {
			Targets     []addin.ClassifiedTarget `json:"targets"`
			Manifest    *addin.Manifest          `json:"manifest,omitempty"`
			HasManifest bool                     `json:"hasManifest"`
		}{
			Targets:     out,
			Manifest:    manifest,
			HasManifest: manifest != nil,
		},
	)
}
