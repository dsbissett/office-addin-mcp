package macrotool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/recorder"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// MakeMacroTool creates a replay tool for the given macro. The returned tool,
// when Run, dispatches each recorded entry sequentially through the dispatcher.
// This requires the dispatcher to be available at runtime, which is complex.
// For now, return a stub that documents the macro structure but requires
// a runner callback to actually execute.
func MakeMacroTool(macro *recorder.Macro, runner func(context.Context, string, json.RawMessage, *tools.RunEnv) tools.Result) tools.Tool {
	// Collect all unique params schemas observed during recording.
	// For v1, we just allow additionalProperties: true since the recorded
	// params are literal.
	schema := json.RawMessage(`{"type":"object","additionalProperties":true}`)

	summary := fmt.Sprintf("Recorded macro with %d steps", len(macro.Entries))

	return tools.Tool{
		Name:        fmt.Sprintf("macro.%s", macro.Name),
		Description: fmt.Sprintf("Replay recorded macro: %s. %s", macro.Name, summary),
		Schema:      schema,
		NoSession:   false, // Replay tools need sessions to execute recorded calls
		Run: func(ctx context.Context, params json.RawMessage, env *tools.RunEnv) tools.Result {
			// Replay each recorded entry in sequence.
			for i, entry := range macro.Entries {
				// Marshal the entry params back to JSON.
				entryParams, err := json.Marshal(entry.Params)
				if err != nil {
					return tools.FailWithDetails(
						tools.CategoryInternal,
						"replay_failed",
						fmt.Sprintf("step %d: marshal params: %v", i, err),
						false,
						map[string]any{"step": i, "tool": entry.Tool},
					)
				}

				// Call the runner to execute this step.
				result := runner(ctx, entry.Tool, entryParams, env)
				if result.Err != nil {
					// Stop on first error and report context.
					details := map[string]any{
						"step":           i,
						"tool":           entry.Tool,
						"stepsCompleted": i,
						"stepsTotal":     len(macro.Entries),
					}
					if result.Err.Details != nil {
						for k, v := range result.Err.Details {
							details[k] = v
						}
					}
					return tools.FailWithDetails(
						result.Err.Category,
						result.Err.Code,
						fmt.Sprintf("step %d (%s) failed: %s", i, entry.Tool, result.Err.Message),
						result.Err.Retryable,
						details,
					)
				}
			}

			// All steps completed successfully.
			return tools.OKWithSummary(
				fmt.Sprintf("Macro completed: %s (%d steps)", macro.Name, len(macro.Entries)),
				map[string]any{
					"macro":         macro.Name,
					"stepsReplayed": len(macro.Entries),
				},
			)
		},
	}
}
