// Package cli holds subcommand implementations. Phase 3 routes the `call`
// subcommand through tools.Dispatch with JSON-Schema-validated params and a
// uniform envelope. PLAN.md §6 lists serve/daemon as later phases.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// CallOptions captures flags for the `call` subcommand.
type CallOptions struct {
	Tool       string
	ParamJSON  string
	BrowserURL string
	WSEndpoint string
	Timeout    time.Duration
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

	env := tools.Dispatch(ctx, reg, tools.Request{
		Tool:   opt.Tool,
		Params: []byte(opt.ParamJSON),
		Endpoint: webview2.Config{
			WSEndpoint: opt.WSEndpoint,
			BrowserURL: opt.BrowserURL,
		},
	})

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
