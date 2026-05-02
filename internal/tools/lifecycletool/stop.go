package lifecycletool

import (
	"context"
	"encoding/json"
	"os"

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
		stopped := len(launch.ListLaunches())
		launch.StopAll()
		if env != nil && env.SetManifest != nil {
			env.SetManifest(nil)
		}
		return tools.OK(map[string]any{
			"stopped": stopped,
			"all":     true,
		})
	}

	manifestPath := p.ManifestPath
	if manifestPath == "" {
		cwd := p.CWD
		if cwd == "" {
			var err error
			cwd, err = os.Getwd()
			if err != nil {
				return tools.Fail(tools.CategoryInternal, "getcwd_failed", err.Error(), false)
			}
		}
		project, err := launch.DetectAddin(cwd)
		if err != nil {
			return tools.FailWithDetails(tools.CategoryNotFound, "addin_not_found", err.Error(), false, map[string]any{
				"cwd": cwd,
			})
		}
		manifestPath = project.ManifestPath
	}

	if _, ok := launch.LookupLaunch(manifestPath); !ok {
		return tools.OK(map[string]any{
			"stopped":      0,
			"manifestPath": manifestPath,
		})
	}
	if err := launch.StopExcel(manifestPath); err != nil {
		le := launch.AsLaunchError(err)
		details := map[string]any{"manifestPath": manifestPath}
		if le != nil {
			details["reason"] = le.Reason
			if len(le.Output) > 0 {
				details["output"] = le.Output
			}
		}
		return tools.FailWithDetails(tools.CategoryInternal, "stop_failed", err.Error(), false, details)
	}
	if env != nil && env.SetManifest != nil && env.Manifest != nil {
		if cur := env.Manifest(); cur != nil && cur.Path == manifestPath {
			env.SetManifest(nil)
		}
	}
	return tools.OK(map[string]any{
		"stopped":      1,
		"manifestPath": manifestPath,
	})
}
