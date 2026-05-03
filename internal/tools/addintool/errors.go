package addintool

import (
	"encoding/json"
	"errors"

	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// mapPayloadError converts an officejs run error to the appropriate envelope
// failure shape. Mirrors exceltool/runner.go so addin.* tools surface
// office.js / protocol failures with consistent categories.
func mapPayloadError(err error) tools.Result {
	var oerr *officejs.OfficeError
	if errors.As(err, &oerr) {
		details := map[string]any{}
		if len(oerr.DebugInfo) > 0 {
			var di any
			if json.Unmarshal(oerr.DebugInfo, &di) == nil {
				details["debugInfo"] = di
			}
		}
		code := oerr.Code
		if code == "" {
			code = "office_js_error"
		}
		res := tools.FailWithDetails(tools.CategoryOfficeJS, code, oerr.Message, false, details)
		res.Err.RecoveryHint = recoveryHintForOfficeCode(code)
		return res
	}
	var pe *officejs.ProtocolException
	if errors.As(err, &pe) {
		return tools.Fail(tools.CategoryProtocol, "payload_protocol_exception", pe.Text, false)
	}
	return tools.ClassifyCDPErr("payload_failed", err)
}

// recoveryHintForOfficeCode returns a short suggestion for the well-known
// codes thrown by internal/js/_preamble.js. Returns "" for codes we don't
// recognize so the agent falls back to the Message.
func recoveryHintForOfficeCode(code string) string {
	switch code {
	case "office_unavailable":
		return "Office.js is not loaded in this target. Confirm the call is targeting an Office add-in taskpane (not the document body), and that the add-in has been sideloaded — call addin.launch if not."
	case "office_ready_failed", "office_ready_timeout":
		return "Office.onReady did not resolve in time — the host may still be initializing. Wait a moment and retry; if persistent, reload the add-in via addin.launch."
	case "requirement_unmet", "requirement_check_failed":
		return "The host does not support the requirement set this tool needs. Check addin.contextInfo for supported requirement sets, and consider an alternate tool that doesn't require this feature."
	}
	return ""
}
