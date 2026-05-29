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
		// Replay re-dispatches arbitrary recorded tool calls (which may include
		// runScript / applyDiff and other writes), so it runs arbitrary code
		// against external Office entities: destructive + open-world.
		Annotations: &tools.Annotations{
			DestructiveHint: tools.BoolPtr(true),
			OpenWorldHint:   tools.BoolPtr(true),
		},
		NoSession: false, // Replay tools need sessions to execute recorded calls
		Run: func(ctx context.Context, params json.RawMessage, env *tools.RunEnv) tools.Result {
			total := len(macro.Entries)
			// Replay each recorded entry in sequence.
			for i, entry := range macro.Entries {
				env.ReportProgress(float64(i), float64(total),
					fmt.Sprintf("step %d/%d: %s", i+1, total, entry.Tool))
				env.Logf("info", "replay %s step %d/%d: %s", macro.Name, i+1, total, entry.Tool)

				if failure, ok := replayStep(ctx, macro, runner, i, entry, env); ok {
					return failure
				}
			}

			// All steps completed successfully.
			env.ReportProgress(float64(total), float64(total), "macro complete")
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

// replayStep executes a single recorded entry. It returns (failure, true) when
// the step should halt replay — either because the params failed to marshal or
// because the runner returned an error — and a zero Result with false when the
// step succeeded and replay should continue.
func replayStep(
	ctx context.Context,
	macro *recorder.Macro,
	runner func(context.Context, string, json.RawMessage, *tools.RunEnv) tools.Result,
	i int,
	entry recorder.Entry,
	env *tools.RunEnv,
) (tools.Result, bool) {
	// Marshal the entry params back to JSON.
	entryParams, err := json.Marshal(entry.Params)
	if err != nil {
		return tools.FailWithDetails(
			tools.CategoryInternal,
			"replay_failed",
			fmt.Sprintf("step %d: marshal params: %v", i, err),
			false,
			map[string]any{"step": i, "tool": entry.Tool},
		), true
	}

	// Call the runner to execute this step.
	result := runner(ctx, entry.Tool, entryParams, env)
	if result.Err != nil {
		// Stop on first error and report context.
		return tools.FailWithDetails(
			result.Err.Category,
			result.Err.Code,
			fmt.Sprintf("step %d (%s) failed: %s", i, entry.Tool, result.Err.Message),
			result.Err.Retryable,
			stepFailureDetails(i, entry.Tool, len(macro.Entries), result.Err.Details),
		), true
	}

	return tools.Result{}, false
}

// stepFailureDetails builds the replay-context details for a failed step,
// merging in any step-level details the runner attached to its error.
func stepFailureDetails(step int, tool string, total int, stepDetails map[string]any) map[string]any {
	details := map[string]any{
		"step":           step,
		"tool":           tool,
		"stepsCompleted": step,
		"stepsTotal":     total,
	}
	for k, v := range stepDetails {
		details[k] = v
	}
	return details
}
