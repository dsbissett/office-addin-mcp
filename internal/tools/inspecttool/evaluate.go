package inspecttool

import (
	"context"
	"encoding/json"

	cdpproto "github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const evaluateSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.evaluate parameters",
  "type": "object",
  "properties": {
    "expression":    {"type": "string", "minLength": 1, "description": "JavaScript expression to evaluate."},
    "awaitPromise":  {"type": "boolean", "description": "Await the resulting promise before returning."},
    "returnByValue": {"type": "boolean", "description": "Return the JSON-serializable value (default true)."},
    "targetId":      {"type": "string"},
    "urlPattern":    {"type": "string"},
    "surface":       {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]}
  },
  "required": ["expression"],
  "additionalProperties": false
}`

type evaluateParams struct {
	Expression    string `json:"expression"`
	AwaitPromise  bool   `json:"awaitPromise,omitempty"`
	ReturnByValue *bool  `json:"returnByValue,omitempty"`
	TargetID      string `json:"targetId,omitempty"`
	URLPattern    string `json:"urlPattern,omitempty"`
	Surface       string `json:"surface,omitempty"`
}

// Evaluate returns the page.evaluate tool — the controlled escape hatch for
// arbitrary JS. Mirrors the legacy cdp.evaluate but participates in the
// Phase 4 surface-aware selector and snapshot model.
func Evaluate() tools.Tool {
	return tools.Tool{
		Name:        "page.evaluate",
		Description: "Run a JS expression in the active page (or the chosen target/surface) via Runtime.evaluate. Use as a controlled escape hatch when no higher-level tool fits.",
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

	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	res, err := att.Conn.Evaluate(ctx, att.SessionID, cdpproto.EvaluateParams{
		Expression:    p.Expression,
		AwaitPromise:  p.AwaitPromise,
		ReturnByValue: returnByValue,
		UserGesture:   true,
	})
	if err != nil {
		return tools.ClassifyCDPErr("evaluate_failed", err)
	}
	if res.ExceptionDetails != nil {
		return tools.Result{
			Err: &tools.EnvelopeError{
				Code:     "evaluation_exception",
				Message:  res.ExceptionDetails.String(),
				Category: tools.CategoryProtocol,
			},
			Summary: "JS evaluation threw: " + res.ExceptionDetails.String(),
		}
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
	return tools.OKWithSummary("Evaluated JS; result type "+out.Type+".", out)
}
