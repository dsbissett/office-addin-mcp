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
		Annotations: &tools.Annotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: tools.BoolPtr(false)},
		Run:         runWaitFor,
	}
}

func runWaitFor(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p waitForParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	timeout := waitForTimeout(p)
	interval := waitForInterval(p)

	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}

	return pollPredicate(ctx, att, p, timeout, interval)
}

// pollPredicate repeatedly evaluates the predicate against the attached target
// until it becomes truthy, the deadline passes, an evaluate error occurs, or
// the context is canceled.
func pollPredicate(ctx context.Context, att *tools.AttachedTarget, p waitForParams, timeout, interval time.Duration) tools.Result {
	deadline := time.Now().Add(timeout)
	for attempts := 1; ; attempts++ {
		if done := pollAttempt(ctx, att, p.Expression, deadline, timeout, attempts); done != nil {
			return *done
		}
		if err := sleepOrCancel(ctx, interval); err != nil {
			return tools.ClassifyCDPErr("wait_canceled", err)
		}
	}
}

// pollAttempt performs a single evaluate-and-classify pass. It returns a
// terminal Result (evaluate error, satisfied, or timed out) or nil to signal
// the caller should sleep and poll again.
func pollAttempt(ctx context.Context, att *tools.AttachedTarget, expression string, deadline time.Time, timeout time.Duration, attempts int) *tools.Result {
	res, err := evaluatePredicate(ctx, att, expression)
	if err != nil {
		return ptrWaitResult(tools.ClassifyCDPErr("evaluate_failed", err))
	}
	if predicateSatisfied(res) {
		return ptrWaitResult(waitSatisfiedResult(attempts))
	}
	if time.Now().After(deadline) {
		return ptrWaitResult(waitTimeoutResult(timeout, attempts))
	}
	return nil
}

// ptrWaitResult boxes a Result for the (*Result == terminal) loop convention.
func ptrWaitResult(r tools.Result) *tools.Result { return &r }

// sleepOrCancel waits for interval to elapse, returning the context error if
// the context is canceled first.
func sleepOrCancel(ctx context.Context, interval time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(interval):
		return nil
	}
}

// evaluatePredicate runs the wrapped, exception-swallowing predicate once.
func evaluatePredicate(ctx context.Context, att *tools.AttachedTarget, expression string) (*cdpproto.EvaluateResult, error) {
	return att.Conn.Evaluate(ctx, att.SessionID, cdpproto.EvaluateParams{
		Expression:    "(function(){ try { return !!(" + expression + "); } catch(e) { return false; } })()",
		ReturnByValue: true,
	})
}

// waitForTimeout resolves the overall timeout, defaulting to 10s.
func waitForTimeout(p waitForParams) time.Duration {
	if d := time.Duration(p.TimeoutMs) * time.Millisecond; d > 0 {
		return d
	}
	return 10 * time.Second
}

// waitForInterval resolves the poll interval, defaulting to 200ms.
func waitForInterval(p waitForParams) time.Duration {
	if d := time.Duration(p.IntervalMs) * time.Millisecond; d > 0 {
		return d
	}
	return 200 * time.Millisecond
}

// predicateSatisfied reports whether the evaluate result is a clean truthy
// value (no exception, a result, and a JSON value of true).
func predicateSatisfied(res *cdpproto.EvaluateResult) bool {
	return res.ExceptionDetails == nil &&
		res.Result != nil &&
		string(res.Result.Value) == "true"
}

// waitSatisfiedResult builds the success envelope for a satisfied predicate.
func waitSatisfiedResult(attempts int) tools.Result {
	return tools.OKWithSummary(
		fmt.Sprintf("Predicate satisfied after %d attempt(s).", attempts),
		struct {
			Satisfied bool `json:"satisfied"`
			Attempts  int  `json:"attempts"`
		}{Satisfied: true, Attempts: attempts},
	)
}

// waitTimeoutResult builds the timeout envelope when the predicate never
// became truthy.
func waitTimeoutResult(timeout time.Duration, attempts int) tools.Result {
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
