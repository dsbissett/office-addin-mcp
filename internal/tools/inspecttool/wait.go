package inspecttool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cdpproto "github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const waitForSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.waitFor parameters",
  "type": "object",
  "properties": {
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]},
    "expression": {"type": "string", "minLength": 1, "description": "JavaScript predicate (truthy = condition satisfied)."},
    "timeoutMs":  {"type": "integer", "minimum": 1, "description": "Overall timeout in ms. Default 10000."},
    "intervalMs": {"type": "integer", "minimum": 1, "description": "Poll interval in ms. Default 200."}
  },
  "required": ["expression"],
  "additionalProperties": false
}`

type waitForParams struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
	Surface    string `json:"surface,omitempty"`
	Expression string `json:"expression"`
	TimeoutMs  int    `json:"timeoutMs,omitempty"`
	IntervalMs int    `json:"intervalMs,omitempty"`
}

// WaitFor returns the page.waitFor tool. It polls Runtime.evaluate against
// the active target with the given predicate until it returns truthy or the
// timeout elapses. Useful for "wait for the dialog to mount" / "wait for the
// table to load" between agent steps.
func WaitFor() tools.Tool {
	return tools.Tool{
		Name:        "page.waitFor",
		Description: "Poll a JS predicate against the active page until it returns truthy or the timeout expires.",
		Schema:      json.RawMessage(waitForSchema),
		Run:         runWaitFor,
	}
}

func runWaitFor(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p waitForParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	timeout := time.Duration(p.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	interval := time.Duration(p.IntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}

	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}

	deadline := time.Now().Add(timeout)
	attempts := 0
	for {
		attempts++
		res, err := att.Conn.Evaluate(ctx, att.SessionID, cdpproto.EvaluateParams{
			Expression:    "(function(){ try { return !!(" + p.Expression + "); } catch(e) { return false; } })()",
			ReturnByValue: true,
		})
		if err != nil {
			return tools.ClassifyCDPErr("evaluate_failed", err)
		}
		if res.ExceptionDetails == nil && res.Result != nil && string(res.Result.Value) == "true" {
			return tools.OKWithSummary(
				fmt.Sprintf("Predicate satisfied after %d attempt(s).", attempts),
				struct {
					Satisfied bool `json:"satisfied"`
					Attempts  int  `json:"attempts"`
				}{Satisfied: true, Attempts: attempts},
			)
		}
		if time.Now().After(deadline) {
			return tools.Result{
				Err: &tools.EnvelopeError{
					Code:      "wait_timeout",
					Message:   "predicate did not become truthy before timeout",
					Category:  tools.CategoryTimeout,
					Retryable: true,
				},
				Summary: fmt.Sprintf("Predicate did not become truthy within %s (%d attempt(s)).", timeout, attempts),
			}
		}
		select {
		case <-ctx.Done():
			return tools.ClassifyCDPErr("wait_canceled", ctx.Err())
		case <-time.After(interval):
		}
	}
}
