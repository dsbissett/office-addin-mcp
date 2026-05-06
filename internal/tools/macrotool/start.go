package macrotool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// RecordStart returns the macro.record_start tool.
func RecordStart() tools.Tool {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "Name of the macro to record (alphanumeric + underscore)"
			}
		},
		"required": ["name"],
		"additionalProperties": false
	}`)

	return tools.Tool{
		Name:        "macro.record_start",
		Description: "Begin recording a new macro with the given name. Subsequent tool calls will be captured until macro.record_stop is called.",
		Schema:      schema,
		NoSession:   true,
		Run: func(ctx context.Context, params json.RawMessage, env *tools.RunEnv) tools.Result {
			type request struct {
				Name string `json:"name"`
			}
			var req request
			if err := json.Unmarshal(params, &req); err != nil {
				return tools.Fail(
					tools.CategoryValidation,
					"invalid_params",
					fmt.Sprintf("parse params: %v", err),
					false,
				)
			}
			if req.Name == "" {
				return tools.Fail(
					tools.CategoryValidation,
					"empty_name",
					"macro name cannot be empty",
					false,
				)
			}

			if env.Recorder == nil {
				return tools.Fail(
					tools.CategoryInternal,
					"recording_unavailable",
					"macro recording is not available in this context",
					false,
				)
			}

			if err := env.Recorder.StartRecording(req.Name); err != nil {
				return tools.Fail(
					tools.CategoryInternal,
					"recording_failed",
					err.Error(),
					false,
				)
			}

			return tools.OKWithSummary(
				fmt.Sprintf("Recording started: %s", req.Name),
				map[string]any{"name": req.Name, "status": "recording"},
			)
		},
	}
}
