// Package officetool provides the shared payload runner used by every
// host-specific Office add-in tool package (exceltool, wordtool, outlooktool,
// powerpointtool, onenotetool). The runner attaches to the selected target,
// dispatches an embedded Office.js payload through internal/officejs, and
// converts outcomes to a tools.Result with optional summary text.
package officetool

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// TargetSelectorBase is the JSON-Schema fragment every host tool embeds in its
// parameter schema so an agent can pick a target by exact id or URL substring.
const TargetSelectorBase = `
    "targetId":   {"type": "string", "description": "Exact target id; mutually exclusive with urlPattern."},
    "urlPattern": {"type": "string", "description": "Substring of the target URL (e.g. add-in taskpane URL)."}
`

// SelectorFields is the embedded struct every host tool's params struct uses
// to pull targetId/urlPattern out of the decoded JSON.
type SelectorFields struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
}

// Selector materializes a tools.TargetSelector from the embedded fields.
func (s SelectorFields) Selector() tools.TargetSelector {
	return tools.TargetSelector{TargetID: s.TargetID, URLPattern: s.URLPattern}
}

// RunPayload attaches to the selected target, dispatches the named Office.js
// payload, and converts the outcome to a tools.Result. summaryFn (optional)
// receives the decoded payload data and returns a one-line human summary
// surfaced via Result.Summary on success. hostLabel ("Excel" / "Word" / …) is
// embedded in failure summaries so chat clients show which host's call broke.
func RunPayload(
	ctx context.Context,
	env *tools.RunEnv,
	sel tools.TargetSelector,
	payload string,
	args any,
	summaryFn func(any) string,
	hostLabel string,
) tools.Result {
	att, err := env.Attach(ctx, sel)
	if err != nil {
		return attachErrToResult(err, hostLabel)
	}

	exec := officejs.New(att.Conn, att.SessionID)
	raw, err := exec.Run(ctx, payload, args)
	if err != nil {
		return payloadErrToResult(err, hostLabel)
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

func codeOrDefault(code string) string {
	if code == "" {
		return "office_js_error"
	}
	return code
}

// attachErrToResult maps an env.Attach failure to a tools.Result, matching the
// shape RunPayload requires: deadline/cancellation flow through ClassifyCDPErr
// (timeout/internal categories), everything else is a not_found attach failure.
func attachErrToResult(err error, hostLabel string) tools.Result {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		res := tools.ClassifyCDPErr("attach_failed", err)
		res.Summary = hostLabel + " attach failed: " + err.Error()
		return res
	}
	return tools.Result{
		Err:     &tools.EnvelopeError{Code: "attach_failed", Message: err.Error(), Category: tools.CategoryNotFound},
		Summary: hostLabel + " attach failed: " + err.Error(),
	}
}

// payloadErrToResult classifies an officejs.Executor.Run error into a
// tools.Result. Office.js errors become office_js failures (with forwarded
// debugInfo), protocol exceptions become payload_protocol_exception/protocol,
// and anything else flows through ClassifyCDPErr as payload_failed. Shared by
// RunPayload and RunDiscover so both surface identical envelopes.
func payloadErrToResult(err error, hostLabel string) tools.Result {
	var oerr *officejs.OfficeError
	if errors.As(err, &oerr) {
		res := tools.FailWithDetails(tools.CategoryOfficeJS, codeOrDefault(oerr.Code), oerr.Message, false, officeErrDetails(oerr))
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
	res.Summary = hostLabel + " payload failed: " + err.Error()
	return res
}

// officeErrDetails extracts the optional debugInfo blob from an OfficeError into
// the Details map FailWithDetails expects. Empty/invalid debug info yields an
// empty (non-nil) map, preserving the existing "Details present, no debugInfo
// key" contract.
func officeErrDetails(oerr *officejs.OfficeError) map[string]any {
	details := map[string]any{}
	if len(oerr.DebugInfo) > 0 {
		var di any
		if json.Unmarshal(oerr.DebugInfo, &di) == nil {
			details["debugInfo"] = di
		}
	}
	return details
}
