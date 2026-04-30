// Package cdptool registers low-level CDP tools (cdp.evaluate, cdp.getTargets,
// cdp.selectTarget) on the shared tools.Registry.
package cdptool

import (
	"context"
	"encoding/json"

	cdpproto "github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const evaluateSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "cdp.evaluate parameters",
  "type": "object",
  "properties": {
    "expression":    {"type": "string", "minLength": 1, "description": "JavaScript expression to evaluate."},
    "awaitPromise":  {"type": "boolean", "description": "Await the resulting promise before returning."},
    "returnByValue": {"type": "boolean", "description": "Return the JSON-serializable value (default true)."},
    "targetId":      {"type": "string", "description": "Exact target id; mutually exclusive with urlPattern."},
    "urlPattern":    {"type": "string", "description": "Substring of the target URL; mutually exclusive with targetId."}
  },
  "required": ["expression"],
  "additionalProperties": false
}`

type evaluateParams struct {
	Expression    string `json:"expression"`
	AwaitPromise  bool   `json:"awaitPromise"`
	ReturnByValue *bool  `json:"returnByValue,omitempty"`
	TargetID      string `json:"targetId,omitempty"`
	URLPattern    string `json:"urlPattern,omitempty"`
}

// Evaluate returns the cdp.evaluate tool definition.
func Evaluate() tools.Tool {
	return tools.Tool{
		Name:        "cdp.evaluate",
		Description: "Run a JavaScript expression in a target page using Runtime.evaluate. Selects a target via targetId, urlPattern, or first-page default.",
		Schema:      json.RawMessage(evaluateSchema),
		Run:         runEvaluate,
	}
}

func runEvaluate(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p evaluateParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	returnByValue := true
	if p.ReturnByValue != nil {
		returnByValue = *p.ReturnByValue
	}

	conn, err := env.OpenConn(ctx)
	if err != nil {
		return tools.Fail(tools.CategoryConnection, "open_failed", err.Error(), true)
	}
	defer conn.Close()

	target, err := tools.ResolveTarget(ctx, conn, tools.TargetSelector{
		TargetID:   p.TargetID,
		URLPattern: p.URLPattern,
	})
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "resolve_target_failed", err.Error(), false)
	}
	env.Diag.TargetID = target.TargetID

	sessionID, err := conn.AttachToTarget(ctx, target.TargetID)
	if err != nil {
		return tools.ClassifyCDPErr("attach_failed", err)
	}
	env.Diag.SessionID = sessionID

	res, err := conn.Evaluate(ctx, sessionID, cdpproto.EvaluateParams{
		Expression:    p.Expression,
		AwaitPromise:  p.AwaitPromise,
		ReturnByValue: returnByValue,
		UserGesture:   true,
	})
	if err != nil {
		return tools.ClassifyCDPErr("evaluate_failed", err)
	}
	if res.ExceptionDetails != nil {
		return tools.Fail(tools.CategoryProtocol, "evaluation_exception",
			res.ExceptionDetails.String(), false)
	}

	out := struct {
		Type        string          `json:"type"`
		Value       json.RawMessage `json:"value,omitempty"`
		Description string          `json:"description,omitempty"`
	}{}
	if res.Result != nil {
		out.Type = res.Result.Type
		out.Value = res.Result.Value
		out.Description = res.Result.Description
	}
	return tools.OK(out)
}
