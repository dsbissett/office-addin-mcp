package inspecttool

import (
	"context"
	"encoding/json"
	"strings"

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
		Description: "Run a JS expression in the active page (or the chosen target/surface) via Runtime.evaluate. Use as a controlled escape hatch when no higher-level tool fits. Requires the add-in to be running: call addin.ensureRunning first — otherwise this fails to attach, or the page's own fetch() calls return \"Failed to fetch\". When awaiting an async function, have it return a value so success can be confirmed (an undefined result is reported as a warning, not proof of success).",
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
		return tools.Result{
			Err: &tools.EnvelopeError{
				Code:         "attach_failed",
				Message:      err.Error(),
				Category:     tools.CategoryNotFound,
				RecoveryHint: "Could not attach to a page target — the add-in is likely not running. Call addin.ensureRunning, then retry.",
				Details:      map[string]any{"recoverableViaTool": "addin.ensureRunning"},
			},
			Summary: "Could not attach to a page target; call addin.ensureRunning first.",
		}
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
		msg := res.ExceptionDetails.String()
		// A "Failed to fetch" thrown by the evaluated script means the page
		// reached the JS runtime fine but could not reach a (usually local)
		// HTTP endpoint — the dev server or add-in backend is down. Translate
		// the opaque stack trace into a directed action instead of surfacing it
		// raw.
		if isFetchFailure(msg) {
			return tools.Result{
				Err: &tools.EnvelopeError{
					Code:         "fetch_failed",
					Message:      msg,
					Category:     tools.CategoryConnection,
					Retryable:    true,
					RecoveryHint: `The evaluated script failed to reach a local endpoint ("Failed to fetch"). The dev server or add-in backend may not be running — call addin.ensureRunning and retry.`,
					Details:      map[string]any{"recoverableViaTool": "addin.ensureRunning"},
				},
				Summary: "JS fetch failed to reach a local endpoint; call addin.ensureRunning and retry.",
			}
		}
		return tools.Result{
			Err: &tools.EnvelopeError{
				Code:     "evaluation_exception",
				Message:  msg,
				Category: tools.CategoryProtocol,
			},
			Summary: "JS evaluation threw: " + msg,
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
	// An awaited promise that resolves to undefined is a frequent false
	// positive: the call "succeeded" but produced no evidence it did the work.
	// Flag it so the caller verifies rather than assuming success.
	if p.AwaitPromise && out.Type == "undefined" {
		return tools.OKWithSummary(
			"Evaluated JS; the awaited promise resolved to undefined — this is not proof the operation succeeded. Have the script return a value, or verify with a follow-up read.",
			out)
	}
	return tools.OKWithSummary("Evaluated JS; result type "+out.Type+".", out)
}

// isFetchFailure reports whether a thrown-exception string is a network-reach
// failure (browser fetch/XHR could not connect), as opposed to an ordinary JS
// error. Covers the common Chromium/WebView2 wordings.
func isFetchFailure(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "failed to fetch") ||
		strings.Contains(m, "networkerror") ||
		strings.Contains(m, "err_connection_refused") ||
		strings.Contains(m, "err_connection_reset") ||
		strings.Contains(m, "load failed")
}
