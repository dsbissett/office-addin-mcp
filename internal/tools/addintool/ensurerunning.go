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
		Description: "START HERE before any page.*, inspect.*, excel.*, or other automation. Probe the WebView2 CDP endpoint and, if unreachable, detect+launch the project under cwd. Returns once the endpoint is reachable. Combines addin.detect + addin.launch into one idempotent call so the agent can recover from a closed Excel without a multi-step dance. Safe to call repeatedly — it no-ops when Excel is already reachable.",
		Schema:      json.RawMessage(ensureRunningSchema),
		Annotations: &tools.Annotations{
			// Additive: launching Excel / a dev server adds processes; it
			// neither overwrites user data nor stops the app. Safe to re-run
			// (it no-ops when already reachable).
			IdempotentHint:  true,
			DestructiveHint: tools.BoolPtr(false),
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
	cwd, fail := resolveCWD(p.CWD)
	if fail != nil {
		return *fail
	}

	env.Logf("info", "detecting add-in project in %s", cwd)
	env.ReportProgress(0, 0, "Detecting add-in project")
	project, detectErr := launch.DetectAddin(cwd)
	// Detection failure isn't fatal yet — if Excel is already running with the
	// debug port we don't need a manifest. Hold onto the error in case the
	// probe also fails.

	// Stream each check as a progress notification. total stays 0 (unknown):
	// the probe may short-circuit ("preexisting") or fall through to a launch
	// with a variable number of phases. The monotonic step counter still gives
	// the client forward motion to render.
	var step float64
	res, source, err := launch.LaunchIfNeeded(ctx, project, launch.LaunchOptions{
		Port:             p.Port,
		Timeout:          time.Duration(p.TimeoutMs) * time.Millisecond,
		DevServerTimeout: time.Duration(p.DevServerTimeoutMs) * time.Millisecond,
		SkipDevServer:    p.SkipDevServer,
		Progress: func(msg string) {
			step++
			env.Logf("info", "%s", msg)
			env.ReportProgress(step, 0, msg)
		},
	})
	if err != nil {
		// LaunchIfNeeded only reaches LaunchExcel when project != nil, so a
		// nil project + probe miss surfaces here as the
		// "no project supplied" LaunchError. Translate to a friendlier
		// addin_not_found shape with a recovery hint.
		if project == nil {
			return ensureNotFoundResult(detectErr, cwd)
		}
		return launchErrToResult(err)
	}

	env.ReportProgress(step+1, 0, "Excel reachable")
	applyLaunchSideEffects(env, res, source)
	return tools.OKWithSummary(ensureRunningSummary(res, source), ensureRunningOutput(res, source))
}

// resolveCWD returns the explicit cwd or the process working directory. On
// failure it returns a non-nil *tools.Result the caller should return verbatim.
func resolveCWD(explicit string) (string, *tools.Result) {
	if explicit != "" {
		return explicit, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		res := tools.Fail(tools.CategoryInternal, "getcwd_failed", err.Error(), false)
		return "", &res
	}
	return cwd, nil
}

// ensureNotFoundResult builds the friendly addin_not_found result for the case
// where Excel is unreachable and no add-in project was detected under cwd.
func ensureNotFoundResult(detectErr error, cwd string) tools.Result {
	return tools.Result{
		Err: &tools.EnvelopeError{
			Code:         "addin_not_found",
			Message:      detectErrMessage(detectErr, cwd),
			Category:     tools.CategoryNotFound,
			Retryable:    false,
			RecoveryHint: "Excel is not reachable on the CDP port and no add-in project was found under cwd. Pass cwd=<add-in project root>, or call addin.detect to locate one, then addin.launch.",
			Details: map[string]any{
				"cwd":                cwd,
				"recoverableViaTool": "addin.detect",
			},
		},
		Summary: "Excel unreachable and no add-in project found under " + cwd + ".",
	}
}

// applyLaunchSideEffects publishes the resolved endpoint, drops pooled sessions
// on a fresh spawn, and loads the manifest when one was located.
func applyLaunchSideEffects(env *tools.RunEnv, res *launch.LaunchResult, source string) {
	if env.SetEndpoint != nil {
		env.SetEndpoint(webview2.Config{BrowserURL: res.CDPURL})
	}
	// A fresh spawn means the old Excel is gone; drop pooled sessions so a
	// reconnect budget burned against the dead endpoint doesn't block the next
	// page op. A "preexisting" hit means the connection was fine all along, so
	// leave sessions untouched.
	if source == "launched" && env.ResetSessions != nil {
		env.ResetSessions()
	}
	loadLaunchedManifest(env, res.ManifestPath)
}

// loadLaunchedManifest parses and publishes the manifest at path when both a
// setter and a non-empty path are present; parse failures are silently ignored.
func loadLaunchedManifest(env *tools.RunEnv, path string) {
	if env.SetManifest == nil || path == "" {
		return
	}
	if m, perr := addin.ParseManifest(path); perr == nil {
		env.SetManifest(m)
	}
}

// ensureRunningOutput assembles the structured result payload, omitting the
// optional devServerPort / output fields when empty.
func ensureRunningOutput(res *launch.LaunchResult, source string) map[string]any {
	out := map[string]any{
		"source":       source, // "preexisting" or "launched"
		"cdpUrl":       res.CDPURL,
		"manifestPath": res.ManifestPath,
		"pid":          res.PID,
		"cdpVerified":  res.CDPVerified,
	}
	if res.DevServerPort > 0 {
		out["devServerPort"] = res.DevServerPort
	}
	if len(res.Output) > 0 {
		out["output"] = res.Output
	}
	return out
}

func ensureRunningSummary(res *launch.LaunchResult, source string) string {
	switch source {
	case "preexisting":
		return fmt.Sprintf("Excel already reachable at %s.", res.CDPURL)
	case "launched":
		return fmt.Sprintf("Launched Excel (pid=%d) at %s.", res.PID, res.CDPURL)
	default:
		return fmt.Sprintf("Excel reachable at %s (source=%s).", res.CDPURL, source)
	}
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
		return tools.Result{
			Err:     &tools.EnvelopeError{Code: "launch_failed", Message: err.Error(), Category: tools.CategoryInternal},
			Summary: "Launch failed: " + err.Error(),
		}
	}
	m := launchReasonMapping(le.Reason)
	details := map[string]any{"reason": le.Reason}
	if len(le.Output) > 0 {
		details["output"] = le.Output
	}
	res := tools.FailWithDetails(m.category, codeFromReason(le.Reason), le.Message, m.retryable, details)
	res.Err.RecoveryHint = m.hint
	res.Summary = "Launch failed: " + le.Message
	return res
}

// launchReasonMap maps a LaunchError reason to the envelope category, retry
// flag, and recovery hint. Reasons absent from the map (incl. the empty
// reason and ReasonLaunchFailed) fall back to the internal/non-retryable
// default returned by launchReasonMapping.
type launchReason struct {
	category  string
	retryable bool
	hint      string
}

var launchReasonMap = map[string]launchReason{
	launch.ReasonUnsupportedPlatform: {
		category: tools.CategoryUnsupported,
		hint:     "WebView2 sideloading is Windows-only. On macOS / Linux, target a headless Chrome via --browser-url instead.",
	},
	launch.ReasonLauncherMissing: {
		category: tools.CategoryUnsupported,
		hint:     "office-addin-debugging is not on PATH. Install it as a devDependency in the add-in project, or make npx available on PATH.",
	},
	launch.ReasonPortAlreadyConfig: {
		category: tools.CategoryUnsupported,
		hint:     `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS already pins --remote-debugging-port. Unset it (or close the Excel that already opened with it) and retry.`,
	},
	launch.ReasonCDPNotReady: {
		category:  tools.CategoryTimeout,
		retryable: true,
		hint:      "Excel started but its dev server / CDP port did not come up in time. Retry with a longer timeoutMs / devServerTimeoutMs.",
	},
	launch.ReasonDevServerNotReady: {
		category:  tools.CategoryTimeout,
		retryable: true,
		hint:      "Excel started but its dev server / CDP port did not come up in time. Retry with a longer timeoutMs / devServerTimeoutMs.",
	},
}

func launchReasonMapping(reason string) launchReason {
	if m, ok := launchReasonMap[reason]; ok {
		return m
	}
	return launchReason{category: tools.CategoryInternal}
}

func codeFromReason(reason string) string {
	if reason == "" {
		return "launch_failed"
	}
	return fmt.Sprintf("launch_%s", reason)
}
