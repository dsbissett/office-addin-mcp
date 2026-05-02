package addintool

import (
	"context"
	"encoding/json"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

const statusSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.status parameters",
  "type": "object",
  "properties": {
    "includeInternal": {"type": "boolean", "description": "Include chrome://, edge://, devtools:// targets in the result. Default false."}
  },
  "additionalProperties": false
}`

const statusOutputSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "addin.status result",
  "type": "object",
  "required": ["endpoint", "manifest", "recoveryHints"],
  "properties": {
    "endpoint": {
      "type": "object",
      "required": ["reachable"],
      "properties": {
        "source":     {"type": "string"},
        "browserUrl": {"type": "string"},
        "wsUrl":      {"type": "string"},
        "reachable":  {"type": "boolean"},
        "error":      {"type": "string"}
      }
    },
    "manifest": {
      "type": "object",
      "required": ["loaded"],
      "properties": {
        "loaded":      {"type": "boolean"},
        "id":          {"type": "string"},
        "displayName": {"type": "string"},
        "path":        {"type": "string"},
        "hosts":       {"type": "array", "items": {"type": "string"}}
      }
    },
    "targets": {
      "type": "array",
      "items": {"type": "object"}
    },
    "recoveryHints": {
      "type": "array",
      "items": {"type": "string"}
    }
  }
}`

type statusParams struct {
	IncludeInternal bool `json:"includeInternal,omitempty"`
}

// statusEndpoint is the discovered-endpoint summary returned to the agent.
type statusEndpoint struct {
	Source     string `json:"source,omitempty"`
	BrowserURL string `json:"browserUrl,omitempty"`
	WSURL      string `json:"wsUrl,omitempty"`
	Reachable  bool   `json:"reachable"`
	Error      string `json:"error,omitempty"`
}

// statusManifest is the active-manifest summary returned to the agent.
type statusManifest struct {
	Loaded      bool     `json:"loaded"`
	ID          string   `json:"id,omitempty"`
	DisplayName string   `json:"displayName,omitempty"`
	Path        string   `json:"path,omitempty"`
	Hosts       []string `json:"hosts,omitempty"`
}

// statusOutput is the structured envelope the AI reads to decide whether to
// retry, relaunch, or surface a hard error to the user.
type statusOutput struct {
	Endpoint      statusEndpoint           `json:"endpoint"`
	Manifest      statusManifest           `json:"manifest"`
	Targets       []addin.ClassifiedTarget `json:"targets,omitempty"`
	RecoveryHints []string                 `json:"recoveryHints"`
}

// Status returns the addin.status tool. It probes the configured endpoint,
// lists CDP targets when reachable, summarizes the active manifest, and
// returns a recoveryHints[] string array the agent can act on. Always
// returns OK — failures are encoded inside the structured payload so the
// agent can read both reachability state and recovery suggestions in one
// call instead of inferring from envelope.error.
func Status() tools.Tool {
	return tools.Tool{
		Name:         "addin.status",
		Title:        "Add-in Status",
		Description:  "Aggregate health snapshot: which CDP endpoint we resolved, whether it's reachable, the live target list, the active manifest, and recoveryHints[] for any missing piece. The first call agents should make on a fresh shell.",
		Schema:       json.RawMessage(statusSchema),
		OutputSchema: json.RawMessage(statusOutputSchema),
		Annotations: &tools.Annotations{
			ReadOnlyHint:    true,
			IdempotentHint:  true,
			DestructiveHint: tools.BoolPtr(false),
		},
		NoSession: true,
		Run:       runStatus,
	}
}

func runStatus(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p statusParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}

	out := statusOutput{
		Manifest:      manifestSummary(env),
		RecoveryHints: []string{},
	}

	ep, err := webview2.Discover(ctx, env.Endpoint)
	if err != nil {
		out.Endpoint = statusEndpoint{
			BrowserURL: env.Endpoint.BrowserURL,
			WSURL:      env.Endpoint.WSEndpoint,
			Reachable:  false,
			Error:      err.Error(),
		}
		out.RecoveryHints = append(out.RecoveryHints,
			`No CDP endpoint reachable. Confirm Excel is running with WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS="--remote-debugging-port=9222", or call addin.ensureRunning to detect and launch the project.`)
		if !out.Manifest.Loaded {
			out.RecoveryHints = append(out.RecoveryHints,
				"No add-in manifest is loaded yet — call addin.detect (or addin.launch) before issuing excel.* / page.* tools.")
		}
		return tools.OK(out)
	}

	out.Endpoint = statusEndpoint{
		Source:     string(ep.Source),
		BrowserURL: ep.BrowserURL,
		WSURL:      ep.WSURL,
		Reachable:  true,
	}

	conn, derr := cdp.Dial(ctx, ep.WSURL)
	if derr != nil {
		out.RecoveryHints = append(out.RecoveryHints,
			"Discovered the CDP endpoint but the WebSocket dial failed. The endpoint may be transitioning — retry in a moment.")
		return tools.OK(out)
	}
	defer func() { _ = conn.Close() }()

	targets, terr := conn.GetTargets(ctx)
	if terr != nil {
		out.RecoveryHints = append(out.RecoveryHints,
			"Connected to the browser but Target.getTargets failed. The browser may be in a transient state — retry, or restart Excel.")
		return tools.OK(out)
	}

	var manifest *addin.Manifest
	if env.Manifest != nil {
		manifest = env.Manifest()
	}
	classified := addin.ClassifyTargets(targets, manifest)
	visible := classified[:0]
	for _, c := range classified {
		if !p.IncludeInternal && tools.IsInternalURL(c.URL) {
			continue
		}
		visible = append(visible, c)
	}
	out.Targets = visible

	if len(visible) == 0 {
		out.RecoveryHints = append(out.RecoveryHints,
			"No add-in targets visible. The taskpane may not have opened yet — verify the add-in is loaded in Excel, or set includeInternal=true to inspect every target.")
	}
	if !out.Manifest.Loaded {
		out.RecoveryHints = append(out.RecoveryHints,
			"No add-in manifest is loaded — call addin.detect or addin.launch so subsequent tools can match targets by surface.")
	}
	return tools.OK(out)
}

// manifestSummary projects the active manifest (if any) into the wire shape
// addin.status returns. Pulled out for clarity; nil-safe.
func manifestSummary(env *tools.RunEnv) statusManifest {
	if env == nil || env.Manifest == nil {
		return statusManifest{}
	}
	m := env.Manifest()
	if m == nil {
		return statusManifest{}
	}
	return statusManifest{
		Loaded:      true,
		ID:          m.ID,
		DisplayName: m.DisplayName,
		Path:        m.Path,
		Hosts:       m.Hosts,
	}
}
