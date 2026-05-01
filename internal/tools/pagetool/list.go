package pagetool

import (
	"context"
	"encoding/json"

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
	var manifest *addin.Manifest
	if env.Manifest != nil {
		manifest = env.Manifest()
	}
	classified := addin.ClassifyTargets(targets, manifest)

	out := make([]addin.ClassifiedTarget, 0, len(classified))
	for _, c := range classified {
		if c.Type != "page" {
			continue
		}
		if !p.IncludeInternal && tools.IsInternalURL(c.URL) {
			continue
		}
		out = append(out, c)
	}
	return tools.OK(struct {
		Pages       []addin.ClassifiedTarget `json:"pages"`
		HasManifest bool                     `json:"hasManifest"`
	}{Pages: out, HasManifest: manifest != nil})
}
