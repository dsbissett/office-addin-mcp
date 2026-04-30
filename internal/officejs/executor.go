package officejs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

// Evaluator is the subset of *cdp.Connection that the executor needs. The
// interface lets tests inject a mock without spinning up a real WS server.
type Evaluator interface {
	Evaluate(ctx context.Context, sessionID string, p cdp.EvaluateParams) (*cdp.EvaluateResult, error)
}

// OfficeError is returned when a payload signaled `__officeError`. Tools map
// this to category=office_js. DebugInfo is the raw JSON returned by the
// payload (often Excel's `e.debugInfo` object) — tools forward it to callers
// untouched so agents can inspect specific Office.js error fields.
type OfficeError struct {
	Code      string
	Message   string
	DebugInfo json.RawMessage
}

func (e *OfficeError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("office.js: %s (%s)", e.Message, e.Code)
	}
	return fmt.Sprintf("office.js: %s", e.Code)
}

// ProtocolException is returned when Runtime.evaluate reported
// exceptionDetails — i.e. the JS itself failed to parse or threw outside the
// payload's own try/catch. Tools generally surface this as category=protocol.
type ProtocolException struct{ Text string }

func (e *ProtocolException) Error() string { return "office.js protocol exception: " + e.Text }

// Executor runs Excel.js payloads inside a CDP session.
type Executor struct {
	eval      Evaluator
	sessionID string
}

// New constructs an Executor bound to a session.
func New(eval Evaluator, sessionID string) *Executor {
	return &Executor{eval: eval, sessionID: sessionID}
}

// Run looks up the payload by tool name, builds the wrapped expression, and
// evaluates it. On payload-success returns the unwrapped `result` JSON; on
// payload-failure returns *OfficeError; on transport/JS-parse failure returns
// *ProtocolException or the underlying transport error.
func (e *Executor) Run(ctx context.Context, toolName string, args any) (json.RawMessage, error) {
	body, err := getPayload(toolName)
	if err != nil {
		return nil, err
	}
	pre, err := preamble()
	if err != nil {
		return nil, err
	}
	argsJSON, err := encodeArgs(args)
	if err != nil {
		return nil, err
	}
	expr := buildExpression(pre, body, argsJSON)

	res, err := e.eval.Evaluate(ctx, e.sessionID, cdp.EvaluateParams{
		Expression:    expr,
		AwaitPromise:  true,
		ReturnByValue: true,
		UserGesture:   true,
	})
	if err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		return nil, &ProtocolException{Text: res.ExceptionDetails.String()}
	}
	if res.Result == nil || len(res.Result.Value) == 0 {
		var t string
		if res.Result != nil {
			t = res.Result.Type
		}
		return nil, fmt.Errorf("officejs: payload returned empty value (type=%q)", t)
	}

	var envelope struct {
		Result      json.RawMessage `json:"result,omitempty"`
		OfficeError bool            `json:"__officeError,omitempty"`
		Code        string          `json:"code,omitempty"`
		Message     string          `json:"message,omitempty"`
		DebugInfo   json.RawMessage `json:"debugInfo,omitempty"`
	}
	if err := json.Unmarshal(res.Result.Value, &envelope); err != nil {
		return nil, fmt.Errorf("officejs: decode payload envelope: %w", err)
	}
	if envelope.OfficeError {
		return nil, &OfficeError{
			Code:      envelope.Code,
			Message:   envelope.Message,
			DebugInfo: envelope.DebugInfo,
		}
	}
	return envelope.Result, nil
}

// encodeArgs JSON-marshals args. encoding/json's default Marshal already
// escapes U+2028 and U+2029 to JS-safe escape sequences — exactly what we
// want, since those codepoints would otherwise terminate a JS string literal.
// HTMLEscape (<, >, &) is left on; harmless inside a parenthesized expression.
func encodeArgs(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("officejs: marshal args: %w", err)
	}
	return string(b), nil
}

const exprTemplate = `(async (args) => {
  try {
%s
%s
  } catch (e) {
    if (e && e.__officeError) {
      return { __officeError: true, code: e.code, message: e.message, debugInfo: e.debugInfo };
    }
    return { __officeError: true, code: 'unhandled_exception', message: (e && e.message) || String(e), debugInfo: { stack: e && e.stack } };
  }
})(%s)`

func buildExpression(preambleSrc, payloadBody, argsJSON string) string {
	return fmt.Sprintf(exprTemplate, preambleSrc, payloadBody, argsJSON)
}
