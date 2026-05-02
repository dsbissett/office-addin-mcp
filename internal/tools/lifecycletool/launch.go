package lifecycletool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/launch"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

const launchSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.launch parameters",
  "type": "object",
  "properties": {
    "cwd":             {"type": "string",  "description": "Working directory to detect from. Defaults to the server's process cwd."},
    "port":            {"type": "integer", "minimum": 1, "maximum": 65535, "description": "WebView2 remote debugging port. Defaults to 9222."},
    "skipDevServer":   {"type": "boolean", "description": "Skip auto-spawning the project's dev-server script."},
    "timeoutMs":       {"type": "integer", "minimum": 1000, "description": "Timeout (ms) waiting for the CDP endpoint to come up. Default 60000."},
    "devServerTimeoutMs": {"type": "integer", "minimum": 1000, "description": "Timeout (ms) waiting for the dev server port to listen. Default 90000."}
  },
  "additionalProperties": false
}`

type launchParams struct {
	CWD                string `json:"cwd,omitempty"`
	Port               int    `json:"port,omitempty"`
	SkipDevServer      bool   `json:"skipDevServer,omitempty"`
	TimeoutMs          int    `json:"timeoutMs,omitempty"`
	DevServerTimeoutMs int    `json:"devServerTimeoutMs,omitempty"`
}

// Launch returns the addin.launch tool. The tool detects the project, spawns
// the dev server (unless skipDevServer is set), runs office-addin-debugging
// to sideload Excel with --remote-debugging-port enabled, and on success
// reconfigures the server's default CDP endpoint so subsequent tool calls
// route to the new Excel automatically.
func Launch() tools.Tool {
	return tools.Tool{
		Name:        "addin.launch",
		Title:       "Launch Add-in",
		Description: "Launch Excel with the detected add-in sideloaded and CDP enabled. Spawns the project's dev server if needed and runs office-addin-debugging start. On success, reconfigures the server's default CDP endpoint to the new launch.",
		Schema:      json.RawMessage(launchSchema),
		Annotations: &tools.Annotations{
			IdempotentHint: true,
			// Spawns Excel + a dev-server process — leave DestructiveHint
			// at the spec default of true so MCP clients can prompt before
			// auto-firing this tool.
		},
		NoSession: true,
		Run:       runLaunch,
	}
}

func runLaunch(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p launchParams
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
		return tools.FailWithDetails(tools.CategoryNotFound, "addin_not_found", err.Error(), false, map[string]any{
			"cwd": cwd,
		})
	}

	res, err := launch.LaunchExcel(ctx, project, launch.LaunchOptions{
		Port:             p.Port,
		Timeout:          time.Duration(p.TimeoutMs) * time.Millisecond,
		DevServerTimeout: time.Duration(p.DevServerTimeoutMs) * time.Millisecond,
		SkipDevServer:    p.SkipDevServer,
	})
	if err != nil {
		return launchErrToResult(err)
	}

	if env.SetEndpoint != nil {
		env.SetEndpoint(webview2.Config{BrowserURL: res.CDPURL})
	}
	if env.SetManifest != nil {
		// Best-effort manifest parse — a launch can succeed even if our
		// extractor fails on an exotic manifest, so swallow parse errors and
		// leave the manifest unset rather than failing the whole call.
		if m, perr := addin.ParseManifest(res.ManifestPath); perr == nil {
			env.SetManifest(m)
		}
	}
	return tools.OK(res)
}

// launchErrToResult maps a *launch.LaunchError onto our envelope categories.
// Reasons that imply user environment misconfiguration (no launcher, port
// already configured, non-Windows host) are non-retryable; transient ones
// (CDP not ready, dev server not ready) are retryable.
func launchErrToResult(err error) tools.Result {
	le := launch.AsLaunchError(err)
	if le == nil {
		return tools.Fail(tools.CategoryInternal, "launch_failed", err.Error(), false)
	}
	var (
		category  = tools.CategoryInternal
		retryable = false
	)
	switch le.Reason {
	case launch.ReasonUnsupportedPlatform, launch.ReasonLauncherMissing, launch.ReasonPortAlreadyConfig:
		category = tools.CategoryUnsupported
	case launch.ReasonCDPNotReady, launch.ReasonDevServerNotReady:
		category = tools.CategoryTimeout
		retryable = true
	}
	details := map[string]any{
		"reason": le.Reason,
	}
	if len(le.Output) > 0 {
		details["output"] = le.Output
	}
	return tools.FailWithDetails(category, codeFromReason(le.Reason), le.Message, retryable, details)
}

func codeFromReason(reason string) string {
	if reason == "" {
		return "launch_failed"
	}
	return fmt.Sprintf("launch_%s", reason)
}
