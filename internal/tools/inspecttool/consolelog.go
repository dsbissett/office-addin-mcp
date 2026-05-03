package inspecttool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const consoleLogSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.consoleLog parameters",
  "type": "object",
  "properties": {
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]},
    "levels":     {"type": "array", "items": {"type": "string"}, "description": "Filter to these console methods (log, info, warn, error, debug, trace) and/or 'exception', 'log.entry'."},
    "sinceSeq":   {"type": "integer", "minimum": 0, "description": "Return entries with seq strictly greater than this. Use the lastSeq from the previous call as a cursor."},
    "limit":      {"type": "integer", "minimum": 1, "maximum": 5000, "description": "Cap the number of returned entries. Default 200."},
    "peek":       {"type": "boolean", "description": "Reserved — drain semantics are caller-driven via sinceSeq, so this is currently a no-op."},
    "clear":      {"type": "boolean", "description": "Empty the buffer (returns no entries)."},
    "maxBuffer":  {"type": "integer", "minimum": 1, "maximum": 100000, "description": "Resize the per-target ring buffer. Default 1000 on first call."}
  },
  "additionalProperties": false
}`

type consoleLogParams struct {
	TargetID   string   `json:"targetId,omitempty"`
	URLPattern string   `json:"urlPattern,omitempty"`
	Surface    string   `json:"surface,omitempty"`
	Levels     []string `json:"levels,omitempty"`
	SinceSeq   int64    `json:"sinceSeq,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	Peek       bool     `json:"peek,omitempty"`
	Clear      bool     `json:"clear,omitempty"`
	MaxBuffer  int      `json:"maxBuffer,omitempty"`
}

const (
	defaultDrainLimit = 200
	defaultMaxBuffer  = 1000
)

// ConsoleLog returns the page.consoleLog tool. It auto-subscribes to
// Runtime.consoleAPICalled, Runtime.exceptionThrown and Log.entryAdded on
// the resolved target on first call, then drains the per-target ring buffer
// using the caller's sinceSeq cursor.
func ConsoleLog() tools.Tool {
	return tools.Tool{
		Name:        "page.consoleLog",
		Description: "Drain buffered console output (console.*, uncaught exceptions, browser log entries) for the active page. Auto-subscribes on first call; Chrome replays existing console messages at subscription time so logs written before the first call are included.",
		Schema:      json.RawMessage(consoleLogSchema),
		Run:         runConsoleLog,
	}
}

func runConsoleLog(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p consoleLogParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	if p.MaxBuffer == 0 {
		p.MaxBuffer = defaultMaxBuffer
	}
	if p.Limit == 0 {
		p.Limit = defaultDrainLimit
	}

	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}

	if err := ensureConsolePump(ctx, env, att.Conn, att.SessionID, p.MaxBuffer); err != nil {
		return tools.ClassifyCDPErr("console_pump_failed", err)
	}

	buf := env.EventBuf(session.ConsoleBufKind, att.SessionID, p.MaxBuffer)
	if p.Clear {
		buf.Clear()
		return tools.OKWithSummary(
			"Cleared console buffer.",
			consoleLogResponse{
				TargetID: att.Target.TargetID,
				Records:  []session.EventRecord{},
			},
		)
	}

	res := buf.Drain(session.DrainOpts{
		SinceSeq: p.SinceSeq,
		Limit:    p.Limit,
		Peek:     p.Peek,
	})
	records := filterConsoleLevels(res.Records, p.Levels)
	suffix := ""
	if res.Dropped {
		suffix = " (buffer overflowed; older entries dropped)"
	}
	return tools.OKWithSummary(
		fmt.Sprintf("Drained %d console record(s)%s.", len(records), suffix),
		consoleLogResponse{
			TargetID: att.Target.TargetID,
			Records:  records,
			LastSeq:  res.LastSeq,
			Dropped:  res.Dropped,
			Capacity: buf.Max(),
		},
	)
}

type consoleLogResponse struct {
	TargetID string                `json:"targetId"`
	Records  []session.EventRecord `json:"records"`
	LastSeq  int64                 `json:"lastSeq,omitempty"`
	Dropped  bool                  `json:"dropped,omitempty"`
	Capacity int                   `json:"capacity,omitempty"`
}

func filterConsoleLevels(records []session.EventRecord, levels []string) []session.EventRecord {
	if len(levels) == 0 {
		return records
	}
	want := make(map[string]struct{}, len(levels))
	for _, l := range levels {
		want[strings.ToLower(l)] = struct{}{}
	}
	out := records[:0]
	for _, r := range records {
		key := strings.ToLower(r.Kind)
		if _, ok := want[key]; ok {
			out = append(out, r)
			continue
		}
		// Allow "log" / "warn" / "error" shorthands to match "console.log" etc.
		if strings.HasPrefix(key, "console.") {
			if _, ok := want[strings.TrimPrefix(key, "console.")]; ok {
				out = append(out, r)
			}
		}
	}
	return out
}
