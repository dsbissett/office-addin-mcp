package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/daemon"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// RunServe is the stdio-mode dispatcher: read newline-delimited JSON
// requests on stdin, write newline-delimited envelopes on stdout. Sessions
// persist across requests for the lifetime of the stream — same session
// reuse benefit as the daemon, but no listener.
func RunServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	stdio := fs.Bool("stdio", false, "read JSON requests on stdin, write envelopes on stdout")
	idleTimeout := fs.Duration("idle-timeout", 30*time.Minute, "GC sessions idle longer than this")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*stdio {
		fmt.Fprintln(stderr, "serve: --stdio is required (other transports not yet implemented)")
		return 2
	}

	reg := DefaultRegistry()
	mgr := session.NewManager(session.Config{IdleTimeout: *idleTimeout})
	defer mgr.Close()
	disp := tools.NewDispatcher(reg, mgr)

	in := bufio.NewReaderSize(os.Stdin, 1<<20)
	for {
		line, err := in.ReadBytes('\n')
		if len(line) > 0 {
			handleStdioLine(line, disp, stdout, stderr)
		}
		if err != nil {
			if err == io.EOF {
				return 0
			}
			fmt.Fprintf(stderr, "serve: read: %v\n", err)
			return 1
		}
	}
}

func handleStdioLine(line []byte, disp *tools.Dispatcher, stdout, stderr io.Writer) {
	var req daemon.CallRequest
	if err := json.Unmarshal(line, &req); err != nil {
		writeServeErr(stdout, "decode_request", err.Error())
		return
	}
	if req.Tool == "" {
		writeServeErr(stdout, "missing_tool", "tool is required")
		return
	}
	ctx := context.Background()
	if req.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutMs)*time.Millisecond)
		defer cancel()
	}
	env := disp.Dispatch(ctx, tools.Request{
		Tool:      req.Tool,
		Params:    req.Params,
		Endpoint:  webview2.Config{WSEndpoint: req.Endpoint.WSEndpoint, BrowserURL: req.Endpoint.BrowserURL},
		SessionID: req.SessionID,
	})
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		fmt.Fprintf(stderr, "serve: encode: %v\n", err)
	}
}

func writeServeErr(w io.Writer, code, msg string) {
	env := tools.Envelope{
		OK: false,
		Error: &tools.EnvelopeError{
			Code:     code,
			Message:  msg,
			Category: tools.CategoryValidation,
		},
		Diagnostics: tools.Diagnostics{EnvelopeVersion: tools.EnvelopeVersion},
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(env)
}
