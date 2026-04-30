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

// runPayload is the shared scaffolding every excel.* tool calls. The
// dispatcher hands us a connection (one-shot or session-pooled) and an
// AttachedTarget; we run the named payload through the Office.js executor
// and translate outcomes to a tools.Result.
func runPayload(ctx context.Context, env *tools.RunEnv, sel tools.TargetSelector, payload string, args any) tools.Result {
	att, err := env.Attach(ctx, sel)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return tools.ClassifyCDPErr("attach_failed", err)
		}
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
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
			return tools.FailWithDetails(tools.CategoryOfficeJS, codeOrDefault(oerr.Code), oerr.Message, false, details)
		}
		var pe *officejs.ProtocolException
		if errors.As(err, &pe) {
			return tools.Fail(tools.CategoryProtocol, "payload_protocol_exception", pe.Text, false)
		}
		return tools.ClassifyCDPErr("payload_failed", err)
	}

	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return tools.Fail(tools.CategoryInternal, "decode_payload_result", err.Error(), false)
	}
	return tools.OK(data)
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
