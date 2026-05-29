package lifecycletool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/launch"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const stopSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.stop parameters",
  "type": "object",
  "properties": {
    "cwd":          {"type": "string", "description": "Working directory whose detected manifest identifies the launch to stop. Defaults to the server's process cwd."},
    "manifestPath": {"type": "string", "description": "Manifest path of a previously launched add-in. Overrides cwd-based detection."},
    "all":          {"type": "boolean", "description": "If true, stop every tracked launch instead of resolving a single one."}
  },
  "additionalProperties": false
}`

type stopParams struct {
	CWD          string `json:"cwd,omitempty"`
	ManifestPath string `json:"manifestPath,omitempty"`
	All          bool   `json:"all,omitempty"`
}

// Stop returns the addin.stop tool. With all=true it tears down every
// tracked launch; otherwise it resolves a single launch via manifestPath or
// by detecting the project at cwd, then runs office-addin-debugging stop.
func Stop() tools.Tool {
	return tools.Tool{
		Name:        "addin.stop",
		Title:       "Stop Add-in",
		Description: "Stop a previously launched Office add-in. Runs office-addin-debugging stop and tears down any dev-server child it spawned. Set all=true to stop every tracked launch.",
		Schema:      json.RawMessage(stopSchema),
		Annotations: &tools.Annotations{
			IdempotentHint:  true,
			DestructiveHint: tools.BoolPtr(true), // explicit: kills child processes
		},
		NoSession: true,
		Run:       runStop,
	}
}

func runStop(_ context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p stopParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}

	if p.All {
		return stopAll(env)
	}

	manifestPath, errRes := resolveStopManifest(p)
	if errRes != nil {
		return *errRes
	}
	return stopSingleLaunch(env, manifestPath)
}

// stopSingleLaunch stops the launch tracked under manifestPath: a no-op success
// when nothing is tracked, an error result when StopExcel fails, otherwise a
// success that also clears the active manifest if it matches.
func stopSingleLaunch(env *tools.RunEnv, manifestPath string) tools.Result {
	if _, ok := launch.LookupLaunch(manifestPath); !ok {
		return tools.OKWithSummary(
			"No tracked launch matched "+manifestPath+".",
			map[string]any{"stopped": 0, "manifestPath": manifestPath},
		)
	}
	if err := launch.StopExcel(manifestPath); err != nil {
		return stopExcelErrToResult(err, manifestPath)
	}
	clearActiveManifest(env, manifestPath)
	return tools.OKWithSummary(
		"Stopped launch for "+manifestPath+".",
		map[string]any{"stopped": 1, "manifestPath": manifestPath},
	)
}

// stopAll tears down every tracked launch and clears the active manifest,
// reporting how many launches were stopped.
func stopAll(env *tools.RunEnv) tools.Result {
	stopped := len(launch.ListLaunches())
	launch.StopAll()
	if env != nil && env.SetManifest != nil {
		env.SetManifest(nil)
	}
	return tools.OKWithSummary(
		fmt.Sprintf("Stopped %d tracked launch(es).", stopped),
		map[string]any{"stopped": stopped, "all": true},
	)
}

// resolveStopManifest determines the manifest path to stop: the explicit
// manifestPath when set, otherwise the manifest of the project detected at cwd
// (defaulting to os.Getwd()). On a getwd or detection failure it returns a
// non-nil *tools.Result the caller should return verbatim.
func resolveStopManifest(p stopParams) (string, *tools.Result) {
	if p.ManifestPath != "" {
		return p.ManifestPath, nil
	}
	cwd, errRes := resolveCwd(p.CWD)
	if errRes != nil {
		return "", errRes
	}
	project, err := launch.DetectAddin(cwd)
	if err != nil {
		res := tools.FailWithDetails(tools.CategoryNotFound, "addin_not_found", err.Error(), false, map[string]any{
			"cwd": cwd,
		})
		return "", &res
	}
	return project.ManifestPath, nil
}

// stopExcelErrToResult maps a StopExcel failure onto an internal stop_failed
// envelope, attaching the LaunchError reason and any captured output.
func stopExcelErrToResult(err error, manifestPath string) tools.Result {
	le := launch.AsLaunchError(err)
	details := map[string]any{"manifestPath": manifestPath}
	if le != nil {
		details["reason"] = le.Reason
		if len(le.Output) > 0 {
			details["output"] = le.Output
		}
	}
	return tools.Result{
		Err: &tools.EnvelopeError{
			Code:     "stop_failed",
			Message:  err.Error(),
			Category: tools.CategoryInternal,
			Details:  details,
		},
		Summary: "Stop failed: " + err.Error(),
	}
}

// clearActiveManifest clears the server's active manifest only when it matches
// the just-stopped manifestPath, leaving an unrelated active manifest intact.
func clearActiveManifest(env *tools.RunEnv, manifestPath string) {
	if activeManifestMatches(env, manifestPath) {
		env.SetManifest(nil)
	}
}

// activeManifestMatches reports whether env exposes a settable active manifest
// whose current value points at manifestPath.
func activeManifestMatches(env *tools.RunEnv, manifestPath string) bool {
	if env == nil || env.SetManifest == nil || env.Manifest == nil {
		return false
	}
	cur := env.Manifest()
	return cur != nil && cur.Path == manifestPath
}
