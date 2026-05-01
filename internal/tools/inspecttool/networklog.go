package inspecttool

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const networkLogSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.networkLog parameters",
  "type": "object",
  "properties": {
    "targetId":         {"type": "string"},
    "urlPattern":       {"type": "string", "description": "Selector for the page target. NOT applied to the request URL filter — see urlMatch."},
    "surface":          {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]},
    "urlMatch":         {"type": "string", "description": "Substring or regex (auto-detected) to filter logged request URLs."},
    "statusMin":        {"type": "integer", "minimum": 0, "description": "Inclusive lower bound on response status."},
    "statusMax":        {"type": "integer", "minimum": 0, "description": "Inclusive upper bound on response status."},
    "failedOnly":       {"type": "boolean", "description": "Return only entries with failed=true."},
    "includeHeaders":   {"type": "boolean", "description": "Include request and response headers in records (omitted by default)."},
    "sinceSeq":         {"type": "integer", "minimum": 0},
    "limit":            {"type": "integer", "minimum": 1, "maximum": 5000, "description": "Cap returned entries. Default 200."},
    "peek":             {"type": "boolean"},
    "clear":            {"type": "boolean"},
    "maxBuffer":        {"type": "integer", "minimum": 1, "maximum": 100000, "description": "Resize the per-target ring buffer. Default 1000 on first call."}
  },
  "additionalProperties": false
}`

type networkLogParams struct {
	TargetID       string `json:"targetId,omitempty"`
	URLPattern     string `json:"urlPattern,omitempty"`
	Surface        string `json:"surface,omitempty"`
	URLMatch       string `json:"urlMatch,omitempty"`
	StatusMin      int    `json:"statusMin,omitempty"`
	StatusMax      int    `json:"statusMax,omitempty"`
	FailedOnly     bool   `json:"failedOnly,omitempty"`
	IncludeHeaders bool   `json:"includeHeaders,omitempty"`
	SinceSeq       int64  `json:"sinceSeq,omitempty"`
	Limit          int    `json:"limit,omitempty"`
	Peek           bool   `json:"peek,omitempty"`
	Clear          bool   `json:"clear,omitempty"`
	MaxBuffer      int    `json:"maxBuffer,omitempty"`
}

// NetworkLog returns the page.networkLog tool. Auto-subscribes to Network.*
// events on first call and emits one correlated record per completed (or
// failed) request.
func NetworkLog() tools.Tool {
	return tools.Tool{
		Name:        "page.networkLog",
		Description: "Drain correlated network request/response records for the active page. Auto-subscribes on first call; in-flight requests are not returned until they complete or fail.",
		Schema:      json.RawMessage(networkLogSchema),
		Run:         runNetworkLog,
	}
}

func runNetworkLog(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p networkLogParams
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

	if err := ensureNetworkPump(ctx, env, att.Conn, att.SessionID, p.MaxBuffer); err != nil {
		return tools.ClassifyCDPErr("network_pump_failed", err)
	}

	buf := env.EventBuf(session.NetworkBufKind, att.SessionID, p.MaxBuffer)
	if p.Clear {
		buf.Clear()
		return tools.OK(networkLogResponse{
			TargetID: att.Target.TargetID,
			Records:  []networkRecord{},
		})
	}

	res := buf.Drain(session.DrainOpts{
		SinceSeq: p.SinceSeq,
		Limit:    p.Limit,
		Peek:     p.Peek,
	})

	matcher, mErr := compileURLMatcher(p.URLMatch)
	if mErr != nil {
		return tools.Fail(tools.CategoryValidation, "url_match_invalid", mErr.Error(), false)
	}

	records, lastFiltered := filterNetworkRecords(res.Records, p, matcher)

	out := networkLogResponse{
		TargetID: att.Target.TargetID,
		Records:  records,
		LastSeq:  res.LastSeq,
		Dropped:  res.Dropped,
		Capacity: buf.Max(),
	}
	// When filtering removes the tail, callers still want a cursor that
	// covers everything they just saw. lastFiltered is the highest seq we
	// actually inspected (filtered or returned), which is a safe cursor.
	if lastFiltered > out.LastSeq {
		out.LastSeq = lastFiltered
	}
	return tools.OK(out)
}

type networkLogResponse struct {
	TargetID string          `json:"targetId"`
	Records  []networkRecord `json:"records"`
	LastSeq  int64           `json:"lastSeq,omitempty"`
	Dropped  bool            `json:"dropped,omitempty"`
	Capacity int             `json:"capacity,omitempty"`
}

type urlMatcher struct {
	re   *regexp.Regexp
	subs string
}

func compileURLMatcher(s string) (*urlMatcher, error) {
	if s == "" {
		return nil, nil
	}
	// Treat strings that contain regex metacharacters as regex; otherwise
	// substring. This keeps "https://contoso.com/api" doing the obvious thing.
	if strings.ContainsAny(s, `^$.*+?()[]{}|\\`) {
		re, err := regexp.Compile(s)
		if err != nil {
			return nil, err
		}
		return &urlMatcher{re: re}, nil
	}
	return &urlMatcher{subs: s}, nil
}

func (m *urlMatcher) match(url string) bool {
	if m == nil {
		return true
	}
	if m.re != nil {
		return m.re.MatchString(url)
	}
	return strings.Contains(url, m.subs)
}

func filterNetworkRecords(in []session.EventRecord, p networkLogParams, m *urlMatcher) ([]networkRecord, int64) {
	out := make([]networkRecord, 0, len(in))
	var lastSeq int64
	for _, r := range in {
		lastSeq = r.Seq
		var rec networkRecord
		if err := json.Unmarshal(r.Data, &rec); err != nil {
			continue
		}
		if !m.match(rec.URL) {
			continue
		}
		if p.FailedOnly && !rec.Failed {
			continue
		}
		if p.StatusMin > 0 && rec.Status < p.StatusMin {
			continue
		}
		if p.StatusMax > 0 && rec.Status > p.StatusMax {
			continue
		}
		if !p.IncludeHeaders {
			rec.ReqHeaders = nil
			rec.RespHeaders = nil
		}
		out = append(out, rec)
	}
	return out, lastSeq
}
