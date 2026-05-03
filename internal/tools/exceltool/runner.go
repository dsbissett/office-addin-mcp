// Package exceltool registers excel.* tools backed by Office.js payloads
// embedded in internal/js. Each tool decodes its params, picks a target via
// the shared selector, attaches, and dispatches the corresponding payload
// through internal/officejs.Executor.
package exceltool

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// targetSelectorBase is the shared block of selector fields embedded in every
// excel.* tool's parameter schema.
const targetSelectorBase = `
    "targetId":   {"type": "string", "description": "Exact target id; mutually exclusive with urlPattern."},
    "urlPattern": {"type": "string", "description": "Substring of the target URL (e.g. add-in taskpane URL)."}
`

// maxCells is the Phase 5 read-truncation cap. Range-reading payloads return
// only the top-left cell of any range whose total cell count exceeds this and
// flag truncated=true; mirrors the reference excel-webview2-mcp behavior.
const maxCells = 1000

// runPayloadSum is the shared scaffolding every excel.* tool calls. The
// dispatcher hands us a connection (one-shot or session-pooled) and an
// AttachedTarget; we run the named payload through the Office.js executor
// and translate outcomes to a tools.Result. summaryFn receives the decoded
// payload data and returns a past-tense one-liner that chat clients surface
// in the tool's OUT bubble. Pass nil to skip summary generation. Failures
// get a generic summary built from the EnvelopeError message.
func runPayloadSum(ctx context.Context, env *tools.RunEnv, sel tools.TargetSelector, payload string, args any, summaryFn func(any) string) tools.Result {
	att, err := env.Attach(ctx, sel)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			res := tools.ClassifyCDPErr("attach_failed", err)
			res.Summary = "Excel attach failed: " + err.Error()
			return res
		}
		return tools.Result{
			Err:     &tools.EnvelopeError{Code: "attach_failed", Message: err.Error(), Category: tools.CategoryNotFound},
			Summary: "Excel attach failed: " + err.Error(),
		}
	}

	exec := officejs.New(att.Conn, att.SessionID)
	raw, err := exec.Run(ctx, payload, args)
	if err != nil {
		var oerr *officejs.OfficeError
		if errors.As(err, &oerr) {
			details := map[string]any{}
			if len(oerr.DebugInfo) > 0 {
				var di any
				if json.Unmarshal(oerr.DebugInfo, &di) == nil {
					details["debugInfo"] = di
				}
			}
			res := tools.FailWithDetails(tools.CategoryOfficeJS, codeOrDefault(oerr.Code), oerr.Message, false, details)
			res.Summary = "Office.js error: " + oerr.Message
			return res
		}
		var pe *officejs.ProtocolException
		if errors.As(err, &pe) {
			return tools.Result{
				Err:     &tools.EnvelopeError{Code: "payload_protocol_exception", Message: pe.Text, Category: tools.CategoryProtocol},
				Summary: "Payload protocol exception: " + pe.Text,
			}
		}
		res := tools.ClassifyCDPErr("payload_failed", err)
		res.Summary = "Excel payload failed: " + err.Error()
		return res
	}

	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return tools.Result{
			Err:     &tools.EnvelopeError{Code: "decode_payload_result", Message: err.Error(), Category: tools.CategoryInternal},
			Summary: "Failed to decode Office.js payload result.",
		}
	}
	res := tools.OK(data)
	if summaryFn != nil {
		res.Summary = summaryFn(data)
	}
	return res
}

// arrayLen returns the length of data[key] when it's a JSON array; otherwise 0.
// Used by list-style summaries that count items in the payload.
func arrayLen(data any, key string) int {
	m, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	arr, ok := m[key].([]any)
	if !ok {
		return 0
	}
	return len(arr)
}

// stringField returns data[key] when it's a string; otherwise "".
func stringField(data any, key string) string {
	m, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// boolField returns data[key] when it's a bool; otherwise false.
func boolField(data any, key string) bool {
	m, ok := data.(map[string]any)
	if !ok {
		return false
	}
	b, _ := m[key].(bool)
	return b
}

func codeOrDefault(code string) string {
	if code == "" {
		return "office_js_error"
	}
	return code
}

type selectorFields struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
}

func (s selectorFields) selector() tools.TargetSelector {
	return tools.TargetSelector{TargetID: s.TargetID, URLPattern: s.URLPattern}
}
