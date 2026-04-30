// Package cli holds subcommand implementations. Phase 5 routes the `call`
// subcommand to a running daemon when one is healthy on the well-known
// socket; otherwise it dispatches in-process exactly as before.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/daemon"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// CallOptions captures flags for the `call` subcommand.
type CallOptions struct {
	Tool       string
	ParamJSON  string
	BrowserURL string
	WSEndpoint string
	SessionID  string
	Timeout    time.Duration
	NoDaemon   bool
}

// RunCall parses flags, builds a tools.Request, and writes the resulting
// envelope to stdout. Exit codes: 0 ok, 1 tool failure, 2 usage error.
func RunCall(args []string, stdout, stderr io.Writer) int {
	return RunCallWith(DefaultRegistry(), args, stdout, stderr)
}

// RunCallWith is RunCall with an injected registry — handy for tests that
// register a fake tool to exercise dispatch behavior in isolation.
func RunCallWith(reg *tools.Registry, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("call", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var opt CallOptions
	fs.StringVar(&opt.Tool, "tool", "", "tool name (e.g. cdp.evaluate)")
	fs.StringVar(&opt.ParamJSON, "param", "{}", "JSON parameters for the tool")
	fs.StringVar(&opt.BrowserURL, "browser-url", "", "Chrome DevTools HTTP endpoint (default: probe http://127.0.0.1:9222)")
	fs.StringVar(&opt.WSEndpoint, "ws-endpoint", "", "Direct browser WebSocket endpoint (overrides --browser-url)")
	fs.StringVar(&opt.SessionID, "session", "", "Phase 5 session id; empty resolves to 'default' in daemon mode")
	fs.DurationVar(&opt.Timeout, "timeout", 30*time.Second, "Tool call timeout")
	fs.BoolVar(&opt.NoDaemon, "no-daemon", false, "Skip daemon autoroute and run in-process")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if opt.Tool == "" {
		fmt.Fprintln(stderr, "call: --tool is required")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), opt.Timeout)
	defer cancel()

	req := tools.Request{
		Tool:   opt.Tool,
		Params: []byte(opt.ParamJSON),
		Endpoint: webview2.Config{
			WSEndpoint: opt.WSEndpoint,
			BrowserURL: opt.BrowserURL,
		},
		SessionID: opt.SessionID,
	}

	var env tools.Envelope
	if !opt.NoDaemon {
		if e, ok := tryDaemonRoute(ctx, opt, req); ok {
			env = e
		}
	}
	if env.Diagnostics.EnvelopeVersion == "" {
		env = tools.Dispatch(ctx, reg, req)
	}

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

// tryDaemonRoute probes the well-known socket file. If a healthy daemon
// answers, the request is routed there and the second return is true.
// Anything else (no socket file, stale socket, daemon error) returns
// (zero, false) so the caller falls through to in-process dispatch.
func tryDaemonRoute(ctx context.Context, opt CallOptions, req tools.Request) (tools.Envelope, bool) {
	info, err := daemon.Probe(ctx, "")
	if err != nil {
		return tools.Envelope{}, false
	}
	dreq := daemon.CallRequest{
		Tool:      req.Tool,
		Params:    req.Params,
		SessionID: req.SessionID,
		Endpoint: daemon.EndpointConfig{
			WSEndpoint: req.Endpoint.WSEndpoint,
			BrowserURL: req.Endpoint.BrowserURL,
		},
		TimeoutMs: int(opt.Timeout / time.Millisecond),
	}
	env, err := daemon.CallDaemon(ctx, info, dreq)
	if err != nil {
		return tools.Envelope{}, false
	}
	return env, true
}
