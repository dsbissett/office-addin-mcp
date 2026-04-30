package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// Dispatch executes one request through the registry: lookup → schema validate
// → run → finalize envelope. It always returns a fully populated Envelope —
// success, validation error, or runtime error.
func Dispatch(ctx context.Context, reg *Registry, req Request) Envelope {
	start := time.Now()
	diag := Diagnostics{Tool: req.Tool, EnvelopeVersion: EnvelopeVersion}

	tool, ok := reg.Get(req.Tool)
	if !ok {
		return finalize(diag, start, Result{Err: &EnvelopeError{
			Code:     "unknown_tool",
			Message:  fmt.Sprintf("unknown tool: %s", req.Tool),
			Category: CategoryNotFound,
		}})
	}

	rawParams := req.Params
	if len(rawParams) == 0 {
		rawParams = []byte("{}")
	}
	if err := validateParams(tool.compiled, rawParams); err != nil {
		return finalize(diag, start, Result{Err: &EnvelopeError{
			Code:     "schema_violation",
			Message:  err.Error(),
			Category: CategoryValidation,
		}})
	}

	env := &RunEnv{
		Diag:     &diag,
		OpenConn: makeOpener(req, &diag),
	}
	res := tool.Run(ctx, rawParams, env)
	return finalize(diag, start, res)
}

func finalize(diag Diagnostics, start time.Time, res Result) Envelope {
	diag.DurationMs = time.Since(start).Milliseconds()
	if res.Err != nil {
		return Envelope{OK: false, Error: res.Err, Diagnostics: diag}
	}
	return Envelope{OK: true, Data: res.Data, Diagnostics: diag}
}

// MarshalEnvelope encodes an envelope to JSON, ensuring Data is rendered as
// JSON (not an opaque any). Returns the bytes ready to write to stdout.
func MarshalEnvelope(env Envelope) ([]byte, error) {
	out, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}
	return out, nil
}

func makeOpener(req Request, diag *Diagnostics) func(context.Context) (*cdp.Connection, error) {
	return func(ctx context.Context) (*cdp.Connection, error) {
		ep, err := webview2.Discover(ctx, req.Endpoint)
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
}
