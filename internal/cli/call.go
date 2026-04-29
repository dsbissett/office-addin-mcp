// Package cli holds subcommand implementations. Phase 1 shipped `call` in
// one-shot mode; Phase 2 adds cdp.getTargets, cdp.selectTarget, and
// browser.navigate, plus a target-selector on cdp.evaluate. PLAN.md §6 lists
// serve/daemon/list-tools/status as later phases.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// Envelope is the uniform tool result shape. PLAN.md §3 formalizes this in
// Phase 3; Phase 1/2 carry the minimum needed for the deliverables.
type Envelope struct {
	OK          bool            `json:"ok"`
	Data        json.RawMessage `json:"data,omitempty"`
	Error       *EnvelopeError  `json:"error,omitempty"`
	Diagnostics Diagnostics     `json:"diagnostics"`
}

// EnvelopeError is the failure payload.
type EnvelopeError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Category  string `json:"category"`
	Retryable bool   `json:"retryable"`
}

// Diagnostics carries observability fields populated by every tool.
type Diagnostics struct {
	Tool       string `json:"tool"`
	SessionID  string `json:"sessionId,omitempty"`
	TargetID   string `json:"targetId,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// CallOptions captures flags for the `call` subcommand.
type CallOptions struct {
	Tool       string
	ParamJSON  string
	BrowserURL string
	WSEndpoint string
	Timeout    time.Duration
}

// RunCall parses flags and dispatches one tool invocation. The envelope is
// written to stdout as a single JSON line. Exit codes: 0 ok, 1 tool failure,
// 2 usage error.
func RunCall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("call", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opt CallOptions
	fs.StringVar(&opt.Tool, "tool", "", "tool name (e.g. cdp.evaluate)")
	fs.StringVar(&opt.ParamJSON, "param", "{}", "JSON parameters for the tool")
	fs.StringVar(&opt.BrowserURL, "browser-url", "", "Chrome DevTools HTTP endpoint (default: probe http://127.0.0.1:9222)")
	fs.StringVar(&opt.WSEndpoint, "ws-endpoint", "", "Direct browser WebSocket endpoint (overrides --browser-url)")
	fs.DurationVar(&opt.Timeout, "timeout", 30*time.Second, "Tool call timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if opt.Tool == "" {
		fmt.Fprintln(stderr, "call: --tool is required")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), opt.Timeout)
	defer cancel()

	env := dispatch(ctx, opt)

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		fmt.Fprintf(stderr, "call: encode envelope: %v\n", err)
		return 1
	}
	if env.OK {
		return 0
	}
	return 1
}

// toolHandler runs one tool. Each handler is responsible for params decode,
// connection setup (if needed), and finalizing the envelope (timing).
type toolHandler func(ctx context.Context, opt CallOptions, params json.RawMessage, diag Diagnostics, start time.Time) Envelope

var handlers = map[string]toolHandler{
	"cdp.evaluate":     runEvaluate,
	"cdp.getTargets":   runGetTargets,
	"cdp.selectTarget": runSelectTarget,
	"browser.navigate": runNavigate,
}

func dispatch(ctx context.Context, opt CallOptions) Envelope {
	start := time.Now()
	diag := Diagnostics{Tool: opt.Tool}
	h, ok := handlers[opt.Tool]
	if !ok {
		return finishFail(diag, start, "not_found", "unknown_tool",
			fmt.Sprintf("unknown tool: %s", opt.Tool), false)
	}
	return h(ctx, opt, []byte(opt.ParamJSON), diag, start)
}

func finishFail(d Diagnostics, start time.Time, category, code, msg string, retryable bool) Envelope {
	d.DurationMs = time.Since(start).Milliseconds()
	return Envelope{
		OK:          false,
		Error:       &EnvelopeError{Code: code, Message: msg, Category: category, Retryable: retryable},
		Diagnostics: d,
	}
}

func finishOK(d Diagnostics, start time.Time, data any) Envelope {
	d.DurationMs = time.Since(start).Milliseconds()
	raw, err := json.Marshal(data)
	if err != nil {
		return finishFail(d, start, "internal", "marshal_data", err.Error(), false)
	}
	return Envelope{OK: true, Data: raw, Diagnostics: d}
}

func classifyCDPErr(diag Diagnostics, start time.Time, code string, err error) Envelope {
	category := "protocol"
	retryable := false
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		category = "timeout"
		code = "timeout"
		retryable = true
	case errors.Is(err, context.Canceled):
		category = "internal"
		code = "canceled"
	case errors.Is(err, cdp.ErrClosed):
		category = "connection"
		retryable = true
	}
	return finishFail(diag, start, category, code, err.Error(), retryable)
}

// targetSelector chooses a CDP target. Empty fields mean "default":
// FirstPageTarget.
type targetSelector struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
}

// resolveTarget picks one target, creating a fresh "about:blank" page only when
// the selector is empty AND no page targets exist (typical headless).
func resolveTarget(ctx context.Context, conn *cdp.Connection, sel targetSelector) (cdp.TargetInfo, error) {
	targets, err := conn.GetTargets(ctx)
	if err != nil {
		return cdp.TargetInfo{}, err
	}
	if sel.TargetID != "" {
		for _, t := range targets {
			if t.TargetID == sel.TargetID {
				return t, nil
			}
		}
		return cdp.TargetInfo{}, fmt.Errorf("no target with targetId %q", sel.TargetID)
	}
	if sel.URLPattern != "" {
		for _, t := range targets {
			if strings.Contains(t.URL, sel.URLPattern) {
				return t, nil
			}
		}
		return cdp.TargetInfo{}, fmt.Errorf("no target with url containing %q", sel.URLPattern)
	}
	if t, ok := cdp.FirstPageTarget(targets); ok {
		return t, nil
	}
	tid, err := conn.CreateTarget(ctx, "about:blank")
	if err != nil {
		return cdp.TargetInfo{}, fmt.Errorf("no page target available and createTarget failed: %w", err)
	}
	return cdp.TargetInfo{TargetID: tid, Type: "page", URL: "about:blank"}, nil
}

// openConn discovers the endpoint and dials. The returned closer is non-nil on
// success; the diagnostics struct is updated with the resolved endpoint URL.
func openConn(ctx context.Context, opt CallOptions, diag *Diagnostics) (*cdp.Connection, error) {
	ep, err := webview2.Discover(ctx, webview2.Config{
		WSEndpoint: opt.WSEndpoint,
		BrowserURL: opt.BrowserURL,
	})
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	if ep.BrowserURL != "" {
		diag.Endpoint = ep.BrowserURL
	} else {
		diag.Endpoint = ep.WSURL
	}
	conn, err := cdp.Dial(ctx, ep.WSURL)
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	return conn, nil
}

// ---------- cdp.evaluate ----------

type evaluateParams struct {
	Expression    string `json:"expression"`
	AwaitPromise  bool   `json:"awaitPromise"`
	ReturnByValue *bool  `json:"returnByValue,omitempty"`
	TargetID      string `json:"targetId,omitempty"`
	URLPattern    string `json:"urlPattern,omitempty"`
}

func runEvaluate(ctx context.Context, opt CallOptions, raw json.RawMessage, diag Diagnostics, start time.Time) Envelope {
	var p evaluateParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return finishFail(diag, start, "validation", "param_decode", err.Error(), false)
	}
	if p.Expression == "" {
		return finishFail(diag, start, "validation", "missing_expression", "expression is required", false)
	}
	returnByValue := true
	if p.ReturnByValue != nil {
		returnByValue = *p.ReturnByValue
	}

	conn, err := openConn(ctx, opt, &diag)
	if err != nil {
		return finishFail(diag, start, "connection", "open_failed", err.Error(), true)
	}
	defer conn.Close()

	target, err := resolveTarget(ctx, conn, targetSelector{TargetID: p.TargetID, URLPattern: p.URLPattern})
	if err != nil {
		category := "not_found"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return classifyCDPErr(diag, start, "resolve_target_failed", err)
		}
		return finishFail(diag, start, category, "resolve_target_failed", err.Error(), false)
	}
	diag.TargetID = target.TargetID

	sessionID, err := conn.AttachToTarget(ctx, target.TargetID)
	if err != nil {
		return classifyCDPErr(diag, start, "attach_failed", err)
	}
	diag.SessionID = sessionID

	res, err := conn.Evaluate(ctx, sessionID, cdp.EvaluateParams{
		Expression:    p.Expression,
		AwaitPromise:  p.AwaitPromise,
		ReturnByValue: returnByValue,
		UserGesture:   true,
	})
	if err != nil {
		return classifyCDPErr(diag, start, "evaluate_failed", err)
	}
	if res.ExceptionDetails != nil {
		return finishFail(diag, start, "protocol", "evaluation_exception",
			res.ExceptionDetails.String(), false)
	}

	out := struct {
		Type        string          `json:"type"`
		Value       json.RawMessage `json:"value,omitempty"`
		Description string          `json:"description,omitempty"`
	}{}
	if res.Result != nil {
		out.Type = res.Result.Type
		out.Value = res.Result.Value
		out.Description = res.Result.Description
	}
	return finishOK(diag, start, out)
}

// ---------- cdp.getTargets ----------

type getTargetsParams struct {
	Type            string `json:"type,omitempty"`            // optional: "page", "iframe", "service_worker", ...
	URLPattern      string `json:"urlPattern,omitempty"`      // substring filter on URL
	IncludeInternal bool   `json:"includeInternal,omitempty"` // include chrome://, edge://, devtools://
}

func runGetTargets(ctx context.Context, opt CallOptions, raw json.RawMessage, diag Diagnostics, start time.Time) Envelope {
	var p getTargetsParams
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &p); err != nil {
			return finishFail(diag, start, "validation", "param_decode", err.Error(), false)
		}
	}

	conn, err := openConn(ctx, opt, &diag)
	if err != nil {
		return finishFail(diag, start, "connection", "open_failed", err.Error(), true)
	}
	defer conn.Close()

	targets, err := conn.GetTargets(ctx)
	if err != nil {
		return classifyCDPErr(diag, start, "get_targets_failed", err)
	}

	filtered := make([]cdp.TargetInfo, 0, len(targets))
	for _, t := range targets {
		if p.Type != "" && t.Type != p.Type {
			continue
		}
		if p.URLPattern != "" && !strings.Contains(t.URL, p.URLPattern) {
			continue
		}
		if !p.IncludeInternal && isInternalURLForCLI(t.URL) {
			continue
		}
		filtered = append(filtered, t)
	}
	return finishOK(diag, start, struct {
		Targets []cdp.TargetInfo `json:"targets"`
	}{Targets: filtered})
}

func isInternalURLForCLI(u string) bool {
	switch {
	case strings.HasPrefix(u, "devtools://"):
		return true
	case strings.HasPrefix(u, "chrome://"):
		return true
	case strings.HasPrefix(u, "edge://"):
		return true
	}
	return false
}

// ---------- cdp.selectTarget ----------

type selectTargetParams struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
}

func runSelectTarget(ctx context.Context, opt CallOptions, raw json.RawMessage, diag Diagnostics, start time.Time) Envelope {
	var p selectTargetParams
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &p); err != nil {
			return finishFail(diag, start, "validation", "param_decode", err.Error(), false)
		}
	}
	if p.TargetID == "" && p.URLPattern == "" {
		return finishFail(diag, start, "validation", "missing_selector",
			"one of targetId or urlPattern is required", false)
	}

	conn, err := openConn(ctx, opt, &diag)
	if err != nil {
		return finishFail(diag, start, "connection", "open_failed", err.Error(), true)
	}
	defer conn.Close()

	target, err := resolveTarget(ctx, conn, targetSelector{TargetID: p.TargetID, URLPattern: p.URLPattern})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return classifyCDPErr(diag, start, "resolve_target_failed", err)
		}
		return finishFail(diag, start, "not_found", "resolve_target_failed", err.Error(), false)
	}
	diag.TargetID = target.TargetID
	return finishOK(diag, start, struct {
		Target cdp.TargetInfo `json:"target"`
	}{Target: target})
}

// ---------- browser.navigate ----------

type navigateParams struct {
	URL        string `json:"url"`
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
}

func runNavigate(ctx context.Context, opt CallOptions, raw json.RawMessage, diag Diagnostics, start time.Time) Envelope {
	var p navigateParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return finishFail(diag, start, "validation", "param_decode", err.Error(), false)
	}
	if p.URL == "" {
		return finishFail(diag, start, "validation", "missing_url", "url is required", false)
	}

	conn, err := openConn(ctx, opt, &diag)
	if err != nil {
		return finishFail(diag, start, "connection", "open_failed", err.Error(), true)
	}
	defer conn.Close()

	target, err := resolveTarget(ctx, conn, targetSelector{TargetID: p.TargetID, URLPattern: p.URLPattern})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return classifyCDPErr(diag, start, "resolve_target_failed", err)
		}
		return finishFail(diag, start, "not_found", "resolve_target_failed", err.Error(), false)
	}
	diag.TargetID = target.TargetID

	sessionID, err := conn.AttachToTarget(ctx, target.TargetID)
	if err != nil {
		return classifyCDPErr(diag, start, "attach_failed", err)
	}
	diag.SessionID = sessionID

	res, err := conn.PageNavigate(ctx, sessionID, p.URL)
	if err != nil {
		return classifyCDPErr(diag, start, "navigate_failed", err)
	}
	if res.ErrorText != "" {
		return finishFail(diag, start, "protocol", "navigate_error", res.ErrorText, false)
	}
	return finishOK(diag, start, struct {
		FrameID  string `json:"frameId"`
		LoaderID string `json:"loaderId,omitempty"`
		URL      string `json:"url"`
	}{FrameID: res.FrameID, LoaderID: res.LoaderID, URL: p.URL})
}
