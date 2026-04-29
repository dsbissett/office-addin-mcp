// Package cli holds subcommand implementations. Phase 1 ships only `call` in
// one-shot mode; PLAN.md §6 lists serve/daemon/list-tools/status as later
// phases.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

// Envelope is the uniform tool result shape. PLAN.md §3 formalizes this in
// Phase 3; Phase 1 carries the minimum needed for the deliverable.
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
	fs.StringVar(&opt.BrowserURL, "browser-url", "http://127.0.0.1:9222", "Chrome DevTools HTTP endpoint")
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

func dispatch(ctx context.Context, opt CallOptions) Envelope {
	start := time.Now()
	diag := Diagnostics{Tool: opt.Tool}

	fail := func(category, code, msg string, retryable bool) Envelope {
		diag.DurationMs = time.Since(start).Milliseconds()
		return Envelope{
			OK:          false,
			Error:       &EnvelopeError{Code: code, Message: msg, Category: category, Retryable: retryable},
			Diagnostics: diag,
		}
	}

	switch opt.Tool {
	case "cdp.evaluate":
		var params struct {
			Expression    string `json:"expression"`
			AwaitPromise  bool   `json:"awaitPromise"`
			ReturnByValue *bool  `json:"returnByValue,omitempty"`
		}
		if err := json.Unmarshal([]byte(opt.ParamJSON), &params); err != nil {
			return fail("validation", "param_decode", err.Error(), false)
		}
		if params.Expression == "" {
			return fail("validation", "missing_expression", "expression is required", false)
		}
		returnByValue := true
		if params.ReturnByValue != nil {
			returnByValue = *params.ReturnByValue
		}

		wsURL := opt.WSEndpoint
		if wsURL == "" {
			v, err := cdp.ResolveBrowserWSURL(ctx, opt.BrowserURL)
			if err != nil {
				return fail("connection", "browser_probe_failed", err.Error(), true)
			}
			wsURL = v
		}

		conn, err := cdp.Dial(ctx, wsURL)
		if err != nil {
			return fail("connection", "ws_dial_failed", err.Error(), true)
		}
		defer conn.Close()

		targets, err := conn.GetTargets(ctx)
		if err != nil {
			return classifyCDPErr(diag, start, "get_targets_failed", err)
		}
		target, ok := cdp.FirstPageTarget(targets)
		if !ok {
			// Headless Chrome may not expose a default page; create one.
			tid, err := conn.CreateTarget(ctx, "about:blank")
			if err != nil {
				return classifyCDPErr(diag, start, "create_target_failed", err)
			}
			target = cdp.TargetInfo{TargetID: tid, Type: "page", URL: "about:blank"}
		}
		diag.TargetID = target.TargetID

		sessionID, err := conn.AttachToTarget(ctx, target.TargetID)
		if err != nil {
			return classifyCDPErr(diag, start, "attach_failed", err)
		}
		diag.SessionID = sessionID

		res, err := conn.Evaluate(ctx, sessionID, cdp.EvaluateParams{
			Expression:    params.Expression,
			AwaitPromise:  params.AwaitPromise,
			ReturnByValue: returnByValue,
			UserGesture:   true,
		})
		if err != nil {
			return classifyCDPErr(diag, start, "evaluate_failed", err)
		}
		if res.ExceptionDetails != nil {
			diag.DurationMs = time.Since(start).Milliseconds()
			return Envelope{
				OK: false,
				Error: &EnvelopeError{
					Code:      "evaluation_exception",
					Message:   res.ExceptionDetails.String(),
					Category:  "protocol",
					Retryable: false,
				},
				Diagnostics: diag,
			}
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
		data, err := json.Marshal(out)
		if err != nil {
			return fail("internal", "marshal_data", err.Error(), false)
		}
		diag.DurationMs = time.Since(start).Milliseconds()
		return Envelope{OK: true, Data: data, Diagnostics: diag}

	default:
		return fail("not_found", "unknown_tool", fmt.Sprintf("unknown tool: %s", opt.Tool), false)
	}
}

func classifyCDPErr(diag Diagnostics, start time.Time, code string, err error) Envelope {
	diag.DurationMs = time.Since(start).Milliseconds()
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
	return Envelope{
		OK:          false,
		Error:       &EnvelopeError{Code: code, Message: err.Error(), Category: category, Retryable: retryable},
		Diagnostics: diag,
	}
}
