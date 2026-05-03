// Package lifecycletool registers the addin.* tools that detect, launch,
// and stop an Office Excel add-in via office-addin-debugging. These tools
// run without a CDP connection — they manage the WebView2 lifecycle that
// every other tool depends on.
package lifecycletool

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/dsbissett/office-addin-mcp/internal/launch"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const detectSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.detect parameters",
  "type": "object",
  "properties": {
    "cwd": {"type": "string", "description": "Working directory to walk upward from. Defaults to the server's process cwd."}
  },
  "additionalProperties": false
}`

type detectParams struct {
	CWD string `json:"cwd,omitempty"`
}

// Detect returns the addin.detect tool. The tool walks up from cwd looking
// for a package.json + Excel manifest pair and reports the project layout
// (paths, package manager, dev-server port).
func Detect() tools.Tool {
	return tools.Tool{
		Name:        "addin.detect",
		Title:       "Detect Add-in Project",
		Description: "Detect an Office Excel add-in project from a working directory. Walks up to 5 levels looking for package.json and a workbook-scoped manifest.{xml,json}. Returns project metadata used by addin.launch.",
		Schema:      json.RawMessage(detectSchema),
		Annotations: &tools.Annotations{
			ReadOnlyHint:    true,
			IdempotentHint:  true,
			DestructiveHint: tools.BoolPtr(false),
		},
		NoSession: true,
		Run:       runDetect,
	}
}

func runDetect(_ context.Context, raw json.RawMessage, _ *tools.RunEnv) tools.Result {
	var p detectParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
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
		if errors.Is(err, launch.ErrNoProject) {
			return tools.Result{
				Err: &tools.EnvelopeError{
					Code:     "addin_not_found",
					Message:  err.Error(),
					Category: tools.CategoryNotFound,
					Details:  map[string]any{"cwd": cwd},
				},
				Summary: "No add-in project found at or above " + cwd + ".",
			}
		}
		return tools.Result{
			Err:     &tools.EnvelopeError{Code: "detect_failed", Message: err.Error(), Category: tools.CategoryInternal},
			Summary: "Detect failed: " + err.Error(),
		}
	}
	return tools.OKWithSummary(
		"Detected "+string(project.ManifestKind)+" add-in at "+project.Root+".",
		project,
	)
}
