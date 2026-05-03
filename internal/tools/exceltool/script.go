package exceltool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const runScriptSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "excel.runScript parameters",
  "description": "Run an arbitrary user-supplied async JS body inside Excel.run. The body sees 'context' (RequestContext) and 'args' (scriptArgs) and must return a JSON-serializable value.",
  "type": "object",
  "properties": {
    "script":     {"type": "string", "minLength": 1, "description": "JS body executed inside Excel.run."},
    "scriptArgs": {"description": "Arbitrary JSON value passed as the 'args' parameter to the script."},` + targetSelectorBase + `},
  "required": ["script"],
  "additionalProperties": false
}`

type runScriptParams struct {
	Script     string          `json:"script"`
	ScriptArgs json.RawMessage `json:"scriptArgs,omitempty"`
	selectorFields
}

// RunScript returns the excel.runScript tool definition.
func RunScript() tools.Tool {
	return tools.Tool{
		Name:        "excel.runScript",
		Description: "Run an arbitrary async JS body inside Excel.run; the body returns a JSON-serializable value. Powerful — agents can compose ad-hoc Excel.js operations without a Go-side tool.",
		Schema:      json.RawMessage(runScriptSchema),
		Run:         runRunScript,
	}
}

func runRunScript(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p runScriptParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	args := map[string]any{"script": p.Script}
	if len(p.ScriptArgs) > 0 {
		args["scriptArgs"] = json.RawMessage(p.ScriptArgs)
	}
	return runPayloadSum(ctx, env, p.selector(), "excel.runScript", args, func(_ any) string {
		return "Ran custom Excel.run script."
	})
}
