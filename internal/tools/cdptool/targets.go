package cdptool

import (
	"context"
	"encoding/json"
	"strings"

	cdpproto "github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const getTargetsSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "cdp.getTargets parameters",
  "type": "object",
  "properties": {
    "type":            {"type": "string", "description": "Filter by target type, e.g. 'page'."},
    "urlPattern":      {"type": "string", "description": "Substring filter on URL."},
    "includeInternal": {"type": "boolean", "description": "Include chrome://, edge://, devtools:// (default false)."}
  },
  "additionalProperties": false
}`

type getTargetsParams struct {
	Type            string `json:"type,omitempty"`
	URLPattern      string `json:"urlPattern,omitempty"`
	IncludeInternal bool   `json:"includeInternal,omitempty"`
}

// GetTargets returns the cdp.getTargets tool definition.
func GetTargets() tools.Tool {
	return tools.Tool{
		Name:        "cdp.getTargets",
		Description: "List CDP targets visible to the browser. Strips internal schemes (chrome://, edge://, devtools://) by default.",
		Schema:      json.RawMessage(getTargetsSchema),
		Run:         runGetTargets,
	}
}

func runGetTargets(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p getTargetsParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}

	conn, err := env.Conn(ctx)
	if err != nil {
		return tools.Fail(tools.CategoryConnection, "open_failed", err.Error(), true)
	}

	targets, err := conn.GetTargets(ctx)
	if err != nil {
		return tools.ClassifyCDPErr("get_targets_failed", err)
	}

	out := make([]cdpproto.TargetInfo, 0, len(targets))
	for _, t := range targets {
		if p.Type != "" && t.Type != p.Type {
			continue
		}
		if p.URLPattern != "" && !strings.Contains(t.URL, p.URLPattern) {
			continue
		}
		if !p.IncludeInternal && tools.IsInternalURL(t.URL) {
			continue
		}
		out = append(out, t)
	}
	return tools.OK(struct {
		Targets []cdpproto.TargetInfo `json:"targets"`
	}{Targets: out})
}

const selectTargetSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "cdp.selectTarget parameters",
  "type": "object",
  "properties": {
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"}
  },
  "anyOf": [
    {"required": ["targetId"]},
    {"required": ["urlPattern"]}
  ],
  "additionalProperties": false
}`

type selectTargetParams struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
}

// SelectTarget returns the cdp.selectTarget tool definition.
func SelectTarget() tools.Tool {
	return tools.Tool{
		Name:        "cdp.selectTarget",
		Description: "Resolve a target by id or URL substring and return its TargetInfo. In session mode this also primes the session selector cache so a subsequent cdp.evaluate hits without a fresh attach.",
		Schema:      json.RawMessage(selectTargetSchema),
		Run:         runSelectTarget,
	}
}

func runSelectTarget(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p selectTargetParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}

	// Use Attach so the result populates the session selector cache; the
	// caller doesn't actually need the flatten session, but priming it makes
	// subsequent evaluate / navigate calls cheaper in daemon mode.
	att, err := env.Attach(ctx, tools.TargetSelector{TargetID: p.TargetID, URLPattern: p.URLPattern})
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "resolve_target_failed", err.Error(), false)
	}
	return tools.OK(struct {
		Target cdpproto.TargetInfo `json:"target"`
	}{Target: att.Target})
}
