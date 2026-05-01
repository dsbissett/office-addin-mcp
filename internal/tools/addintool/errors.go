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
		return tools.FailWithDetails(tools.CategoryOfficeJS, code, oerr.Message, false, details)
	}
	var pe *officejs.ProtocolException
	if errors.As(err, &pe) {
		return tools.Fail(tools.CategoryProtocol, "payload_protocol_exception", pe.Text, false)
	}
	return tools.ClassifyCDPErr("payload_failed", err)
}
