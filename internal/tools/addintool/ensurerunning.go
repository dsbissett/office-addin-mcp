package addintool

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

const ensureRunningSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.ensureRunning parameters",
  "type": "object",
  "properties": {
    "cwd":               {"type": "string",  "description": "Directory to detect the add-in project from. Defaults to the server's process cwd."},
    "port":              {"type": "integer", "minimum": 1, "maximum": 65535, "description": "WebView2 remote debugging port to probe / launch with. Defaults to 9222."},
    "skipDevServer":     {"type": "boolean", "description": "Skip auto-spawning the project's dev-server script on launch."},
    "timeoutMs":         {"type": "integer", "minimum": 1000, "description": "Timeout (ms) waiting for the CDP endpoint to come up. Default 60000."},
    "devServerTimeoutMs":{"type": "integer", "minimum": 1000, "description": "Timeout (ms) waiting for the dev server port to listen. Default 90000."}
  },
  "additionalProperties": false
}`

type ensureRunningParams struct {
	CWD                string `json:"cwd,omitempty"`
	Port               int    `json:"port,omitempty"`
	SkipDevServer      bool   `json:"skipDevServer,omitempty"`
	TimeoutMs          int    `json:"timeoutMs,omitempty"`
	DevServerTimeoutMs int    `json:"devServerTimeoutMs,omitempty"`
}

// EnsureRunning returns the addin.ensureRunning tool. It is the
// "make CDP reachable" entry point an agent should call before driving Excel
// from a fresh shell: probes the configured port and, if nothing is
// listening, detects the add-in project under `cwd` and runs addin.launch
// internally. The agent doesn't need to know which path was taken — the
// returned `source` field is `"preexisting"` or `"launched"`.
func EnsureRunning() tools.Tool {
	return tools.Tool{
		Name:        "addin.ensureRunning",
		Title:       "Ensure Excel Is Running",
		Description: "Probe the WebView2 CDP endpoint and, if unreachable, detect+launch the project under cwd. Returns once the endpoint is reachable. Combines addin.detect + addin.launch into one idempotent call so the agent can recover from a closed Excel without a multi-step dance.",
		Schema:      json.RawMessage(ensureRunningSchema),
		Annotations: &tools.Annotations{
			IdempotentHint: true,
			// May start an Excel process — leave DestructiveHint at the
			// spec default of true so MCP clients can prompt before re-run.
		},
		NoSession: true,
		Run:       runEnsureRunning,
	}
}

func runEnsureRunning(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p ensureRunningParams
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

	project, detectErr := launch.DetectAddin(cwd)
	// Detection failure isn't fatal yet — if Excel is already running with the
	// debug port we don't need a manifest. Hold onto the error in case the
	// probe also fails.

	res, source, err := launch.LaunchIfNeeded(ctx, project, launch.LaunchOptions{
		Port:             p.Port,
		Timeout:          time.Duration(p.TimeoutMs) * time.Millisecond,
		DevServerTimeout: time.Duration(p.DevServerTimeoutMs) * time.Millisecond,
		SkipDevServer:    p.SkipDevServer,
	})
	if err != nil {
		// LaunchIfNeeded only reaches LaunchExcel when project != nil, so a
		// nil project + probe miss surfaces here as the
		// "no project supplied" LaunchError. Translate to a friendlier
		// addin_not_found shape with a recovery hint.
		if project == nil {
			return tools.Result{Err: &tools.EnvelopeError{
				Code:         "addin_not_found",
				Message:      detectErrMessage(detectErr, cwd),
				Category:     tools.CategoryNotFound,
				Retryable:    false,
				RecoveryHint: "Excel is not reachable on the CDP port and no add-in project was found under cwd. Pass cwd=<add-in project root>, or call addin.detect to locate one, then addin.launch.",
				Details: map[string]any{
					"cwd":                cwd,
					"recoverableViaTool": "addin.detect",
				},
			}}
		}
		return launchErrToResult(err)
	}

	if env.SetEndpoint != nil {
		env.SetEndpoint(webview2.Config{BrowserURL: res.CDPURL})
	}
	if env.SetManifest != nil && res.ManifestPath != "" {
		if m, perr := addin.ParseManifest(res.ManifestPath); perr == nil {
			env.SetManifest(m)
		}
	}

	out := map[string]any{
		"source":       source, // "preexisting" or "launched"
		"cdpUrl":       res.CDPURL,
		"manifestPath": res.ManifestPath,
		"pid":          res.PID,
	}
	if res.DevServerPort > 0 {
		out["devServerPort"] = res.DevServerPort
	}
	if len(res.Output) > 0 {
		out["output"] = res.Output
	}
	return tools.OK(out)
}

func detectErrMessage(detectErr error, cwd string) string {
	if detectErr == nil {
		return fmt.Sprintf("no add-in project resolved from %s", cwd)
	}
	return detectErr.Error()
}

// launchErrToResult mirrors lifecycletool's mapping so addin.ensureRunning
// surfaces the same LaunchError reasons (cdp-not-ready, dev-server-not-ready,
// launcher-missing, …) with consistent codes/categories.
func launchErrToResult(err error) tools.Result {
	le := launch.AsLaunchError(err)
	if le == nil {
		return tools.Fail(tools.CategoryInternal, "launch_failed", err.Error(), false)
	}
	var (
		category  = tools.CategoryInternal
		retryable = false
		hint      = ""
	)
	switch le.Reason {
	case launch.ReasonUnsupportedPlatform:
		category = tools.CategoryUnsupported
		hint = "WebView2 sideloading is Windows-only. On macOS / Linux, target a headless Chrome via --browser-url instead."
	case launch.ReasonLauncherMissing:
		category = tools.CategoryUnsupported
		hint = "office-addin-debugging is not on PATH. Install it as a devDependency in the add-in project, or make npx available on PATH."
	case launch.ReasonPortAlreadyConfig:
		category = tools.CategoryUnsupported
		hint = `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS already pins --remote-debugging-port. Unset it (or close the Excel that already opened with it) and retry.`
	case launch.ReasonCDPNotReady, launch.ReasonDevServerNotReady:
		category = tools.CategoryTimeout
		retryable = true
		hint = "Excel started but its dev server / CDP port did not come up in time. Retry with a longer timeoutMs / devServerTimeoutMs."
	}
	details := map[string]any{"reason": le.Reason}
	if len(le.Output) > 0 {
		details["output"] = le.Output
	}
	res := tools.FailWithDetails(category, codeFromReason(le.Reason), le.Message, retryable, details)
	res.Err.RecoveryHint = hint
	return res
}

func codeFromReason(reason string) string {
	if reason == "" {
		return "launch_failed"
	}
	return fmt.Sprintf("launch_%s", reason)
}
