package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/session"
)

// Dispatcher binds a Registry to a session.Manager. In daemon mode the
// Manager is shared across requests so connections persist. In one-shot
// mode (the free Dispatch function below) a fresh ephemeral Manager is
// created per call, so each call dials its own connection.
type Dispatcher struct {
	Registry *Registry
	Sessions *session.Manager
	// Ephemeral makes the dispatcher Drop the session after each call.
	// One-shot callers set this; daemons leave it false so connections
	// persist for reuse.
	Ephemeral bool
}

// NewDispatcher builds a Dispatcher.
func NewDispatcher(reg *Registry, mgr *session.Manager) *Dispatcher {
	return &Dispatcher{Registry: reg, Sessions: mgr}
}

// Dispatch is the historic free function — back-compat one-shot path. Creates
// a private session.Manager per invocation so behavior matches Phase 1–4
// exactly: each call dials, runs, closes.
func Dispatch(ctx context.Context, reg *Registry, req Request) Envelope {
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	d := &Dispatcher{Registry: reg, Sessions: mgr, Ephemeral: true}
	return d.Dispatch(ctx, req)
}

// Dispatch executes one request. Sequence:
//  1. tool lookup
//  2. JSON Schema validation against the tool's Schema
//  3. session acquire (lock + ensure conn within reconnect budget)
//  4. tool Run with helpers wired around the session
//  5. envelope finalize (CDPRoundTrips, DurationMs, EnvelopeVersion)
//  6. ephemeral drop (one-shot mode)
//
// Always returns a fully populated Envelope.
func (d *Dispatcher) Dispatch(ctx context.Context, req Request) Envelope {
	start := time.Now()
	diag := Diagnostics{
		Tool:            req.Tool,
		EnvelopeVersion: EnvelopeVersion,
		SessionID:       req.SessionID,
	}

	tool, ok := d.Registry.Get(req.Tool)
	if !ok {
		return finalize(diag, start, 0, Result{Err: &EnvelopeError{
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
		return finalize(diag, start, 0, Result{Err: &EnvelopeError{
			Code:     "schema_violation",
			Message:  err.Error(),
			Category: CategoryValidation,
		}})
	}

	sess := d.Sessions.Get(req.SessionID)
	if d.Ephemeral {
		defer d.Sessions.Drop(req.SessionID)
	}

	conn, release, err := sess.Acquire(ctx, req.Endpoint)
	if err != nil {
		category := CategoryConnection
		retryable := true
		// reconnect-budget exhaustion is a user-visible terminal state until
		// reset; surface it as non-retryable so callers know to back off.
		if errors.Is(err, context.DeadlineExceeded) {
			category = CategoryTimeout
		}
		return finalize(diag, start, 0, Result{Err: &EnvelopeError{
			Code:      "session_acquire_failed",
			Message:   err.Error(),
			Category:  category,
			Retryable: retryable,
		}})
	}
	defer release()

	// Endpoint diagnostic — populated by the connection lifecycle. We rebuild
	// from the session's view of the endpoint config.
	if ep := req.Endpoint; ep.WSEndpoint != "" {
		diag.Endpoint = ep.WSEndpoint
	} else if ep.BrowserURL != "" {
		diag.Endpoint = ep.BrowserURL
	}

	rtStart := conn.RoundTrips()

	env := buildRunEnv(sess, conn, &diag)
	res := tool.Run(ctx, rawParams, env)

	return finalize(diag, start, conn.RoundTrips()-rtStart, res)
}

func finalize(diag Diagnostics, start time.Time, roundTrips int64, res Result) Envelope {
	diag.DurationMs = time.Since(start).Milliseconds()
	diag.CDPRoundTrips = roundTrips
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

// buildRunEnv wires the per-call helpers around a locked session and live
// connection. The Attach helper consults the session's selection cache so
// repeat calls with the same selector skip Target.getTargets and
// Target.attachToTarget — manifesting as the CDPRoundTrips drop the Phase 5
// deliverable expects.
func buildRunEnv(sess *session.Session, conn *cdp.Connection, diag *Diagnostics) *RunEnv {
	return &RunEnv{
		Diag: diag,
		Conn: func(_ context.Context) (*cdp.Connection, error) {
			return conn, nil
		},
		Attach: func(ctx context.Context, sel TargetSelector) (*AttachedTarget, error) {
			if cached, ok := sess.Selected(sel.TargetID, sel.URLPattern); ok {
				diag.TargetID = cached.Target.TargetID
				diag.CDPSessionID = cached.SessionID
				return &AttachedTarget{
					Conn:      conn,
					Target:    cached.Target,
					SessionID: cached.SessionID,
				}, nil
			}
			target, err := ResolveTarget(ctx, conn, sel)
			if err != nil {
				return nil, err
			}
			diag.TargetID = target.TargetID
			cdpSID, err := conn.AttachToTarget(ctx, target.TargetID)
			if err != nil {
				return nil, err
			}
			diag.CDPSessionID = cdpSID
			sess.SetSelected(sel.TargetID, sel.URLPattern, target, cdpSID)
			return &AttachedTarget{
				Conn:      conn,
				Target:    target,
				SessionID: cdpSID,
			}, nil
		},
	}
}
