package outlooktool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/officetool"
)

const runScriptSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "outlook.runScript parameters",
  "description": "Run an arbitrary user-supplied async JS body with Office.context.mailbox passed as 'mailbox' and the user-supplied scriptArgs as 'args'. Must return a JSON-serializable value.",
  "type": "object",
  "properties": {
    "script":     {"type": "string", "minLength": 1, "description": "JS body executed with mailbox + args."},
    "scriptArgs": {"description": "Arbitrary JSON value passed as the 'args' parameter to the script."},` + targetSelectorBase + `},
  "required": ["script"],
  "additionalProperties": false
}`

type runScriptParams struct {
	Script     string          `json:"script"`
	ScriptArgs json.RawMessage `json:"scriptArgs,omitempty"`
	officetool.SelectorFields
}

// RunScript returns the outlook.runScript tool definition.
func RunScript() tools.Tool {
	return tools.Tool{
		Name:        "outlook.runScript",
		Description: "Run an arbitrary async JS body with Office.context.mailbox; the body returns a JSON-serializable value. Outlook has no batched-context Outlook.run API.",
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
	return runPayloadSum(ctx, env, p.Selector(), "outlook.runScript", args, func(_ any) string {
		return "Ran custom Outlook script."
	})
}
