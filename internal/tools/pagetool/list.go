package pagetool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const listSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "pages.list parameters",
  "type": "object",
  "properties": {
    "includeInternal": {"type": "boolean", "description": "Include chrome://, edge://, devtools:// targets. Default false."}
  },
  "additionalProperties": false
}`

type listParams struct {
	IncludeInternal bool `json:"includeInternal,omitempty"`
}

// List returns the pages.list tool. Filters CDP targets to type=page (skipping
// service workers, custom-functions runtimes, etc.) and labels each with the
// manifest-classified surface so an agent can pick a target by role.
func List() tools.Tool {
	return tools.Tool{
		Name:        "pages.list",
		Description: "List CDP page targets classified by manifest surface. Skips service workers and custom-functions runtimes.",
		Schema:      json.RawMessage(listSchema),
		Run:         runList,
		Annotations: &tools.Annotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: tools.BoolPtr(false)},
	}
}

func runList(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p listParams
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
	manifest := resolveManifest(env)
	classified := addin.ClassifyTargets(targets, manifest)
	out := filterPageTargets(classified, p.IncludeInternal)
	return tools.OKWithSummary(
		fmt.Sprintf("Listed %d page target(s).", len(out)),
		struct {
			Pages       []addin.ClassifiedTarget `json:"pages"`
			HasManifest bool                     `json:"hasManifest"`
		}{Pages: out, HasManifest: manifest != nil},
	)
}

// resolveManifest returns the active manifest, or nil when the env supplies no
// manifest accessor.
func resolveManifest(env *tools.RunEnv) *addin.Manifest {
	if env.Manifest == nil {
		return nil
	}
	return env.Manifest()
}

// filterPageTargets keeps only type=page targets, dropping internal URLs unless
// includeInternal is set. The returned slice is never nil so the envelope
// encodes an empty array rather than null.
func filterPageTargets(classified []addin.ClassifiedTarget, includeInternal bool) []addin.ClassifiedTarget {
	out := make([]addin.ClassifiedTarget, 0, len(classified))
	for _, c := range classified {
		if keepPageTarget(c, includeInternal) {
			out = append(out, c)
		}
	}
	return out
}

// keepPageTarget reports whether a classified target survives the list filter.
func keepPageTarget(c addin.ClassifiedTarget, includeInternal bool) bool {
	if c.Type != "page" {
		return false
	}
	if !includeInternal && tools.IsInternalURL(c.URL) {
		return false
	}
	return true
}
