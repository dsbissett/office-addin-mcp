package macrotool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// RecordStop returns the macro.record_stop tool.
func RecordStop() tools.Tool {
	return tools.Tool{
		Name:        "macro.record_stop",
		Description: "Stop recording the current macro and save it to disk.",
		Schema:      json.RawMessage(`{"type":"object","additionalProperties":false}`),
		NoSession:   true,
		Run: func(ctx context.Context, params json.RawMessage, env *tools.RunEnv) tools.Result {
			if env.Recorder == nil {
				return tools.Fail(
					tools.CategoryInternal,
					"recording_unavailable",
					"macro recording is not available in this context",
					false,
				)
			}

			macro, err := env.Recorder.StopRecording()
			if err != nil {
				return tools.Fail(
					tools.CategoryInternal,
					"recording_failed",
					err.Error(),
					false,
				)
			}

			return tools.OKWithSummary(
				fmt.Sprintf("Recording stopped: %s (%d steps)", macro.Name, len(macro.Entries)),
				map[string]any{
					"name":    macro.Name,
					"steps":   len(macro.Entries),
					"entries": macro.Entries,
				},
			)
		},
	}
}
