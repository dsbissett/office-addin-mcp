package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	internallog "github.com/dsbissett/office-addin-mcp/internal/log"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
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
	// AllowDangerous propagates into RunEnv.AllowDangerous. Set from the
	// process-wide --allow-dangerous-cdp flag / OAMCP_ALLOW_DANGEROUS_CDP
	// env. Off by default — dangerous CDP methods refuse without it.
	AllowDangerous bool
	// SetEndpoint, if non-nil, is wired into RunEnv.SetEndpoint and lets
	// lifecycle tools (addin.launch) reconfigure the server's default CDP
	// endpoint after sideloading Excel.
	SetEndpoint func(webview2.Config)
	// Manifest returns the active manifest if any. Wired into RunEnv.Manifest.
	Manifest func() *addin.Manifest
	// SetManifest stores a manifest at server scope (Phase 3). Wired into
	// RunEnv.SetManifest.
	SetManifest func(*addin.Manifest)
	// DocCache is the cross-session document discovery cache. Wired into
	// every RunEnv. nil falls through to a disabled store at first access.
	DocCache *doccache.Store
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
	requestID := newRequestID()
	ctx = internallog.WithRequestID(ctx, requestID)
	diag := Diagnostics{
		Tool:            req.Tool,
		EnvelopeVersion: EnvelopeVersion,
		RequestID:       requestID,
		SessionID:       req.SessionID,
	}
	slog.Debug("dispatch.start", "request_id", requestID, "tool", req.Tool, "session_id", req.SessionID)
	defer func() {
		slog.Debug("dispatch.end", "request_id", requestID, "tool", req.Tool, "duration_ms", time.Since(start).Milliseconds())
	}()

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

	if tool.NoSession {
		env := &RunEnv{
			Diag:           &diag,
			Endpoint:       req.Endpoint,
			AllowDangerous: d.AllowDangerous,
			SetEndpoint:    d.SetEndpoint,
			Manifest:       d.Manifest,
			SetManifest:    d.SetManifest,
			DocCache:       d.DocCache,
		}
		res := tool.Run(ctx, rawParams, env)
		return finalize(diag, start, 0, res)
	}

	sess := d.Sessions.Get(req.SessionID)
	if d.Ephemeral {
		defer d.Sessions.Drop(req.SessionID)
	}

	conn, release, err := sess.Acquire(ctx, req.Endpoint)
	if err != nil {
		return finalize(diag, start, 0, Result{Err: classifyAcquireErr(err, req.Endpoint)})
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

	env := buildRunEnv(sess, conn, &diag, d.AllowDangerous, d.Manifest)
	env.Endpoint = req.Endpoint
	env.SetEndpoint = d.SetEndpoint
	env.Manifest = d.Manifest
	env.SetManifest = d.SetManifest
	env.DocCache = d.DocCache
	res := tool.Run(ctx, rawParams, env)

	return finalize(diag, start, conn.RoundTrips()-rtStart, res)
}

func finalize(diag Diagnostics, start time.Time, roundTrips int64, res Result) Envelope {
	diag.DurationMs = time.Since(start).Milliseconds()
	diag.CDPRoundTrips = roundTrips
	if res.Err != nil {
		return Envelope{OK: false, Error: res.Err, Summary: res.Summary, Diagnostics: diag}
	}
	return Envelope{OK: true, Data: res.Data, Summary: res.Summary, Diagnostics: diag}
}

// classifyAcquireErr maps a session.Acquire failure to a rich EnvelopeError
// with a code distinct enough for the agent to branch on, a recovery hint,
// and Details["probedEndpoint"]/["recoverableViaTool"] when applicable.
func classifyAcquireErr(err error, ep webview2.Config) *EnvelopeError {
	probed := ep.WSEndpoint
	if probed == "" {
		probed = ep.BrowserURL
	}
	if probed == "" {
		probed = "http://127.0.0.1:9222"
	}
	details := map[string]any{"probedEndpoint": probed}

	switch {
	case errors.Is(err, session.ErrReconnectBudgetExhausted):
		details["recoverableViaTool"] = "addin.launch"
		return &EnvelopeError{
			Code:         "session_reconnect_budget_exhausted",
			Message:      err.Error(),
			Category:     CategoryConnection,
			Retryable:    false,
			RecoveryHint: "Reconnect budget (3 attempts per 60s) is exhausted. Excel may not be running with --remote-debugging-port=9222 — call addin.launch with the manifest, or wait 60 seconds and retry.",
			Details:      details,
		}
	case errors.Is(err, context.DeadlineExceeded):
		return &EnvelopeError{
			Code:         "session_acquire_timeout",
			Message:      err.Error(),
			Category:     CategoryTimeout,
			Retryable:    true,
			RecoveryHint: "Tool call timed out before the CDP connection was ready. Retry with a longer ctx deadline, or call addin.launch if Excel is not running.",
			Details:      details,
		}
	case errors.Is(err, session.ErrDialFailed):
		details["recoverableViaTool"] = "addin.launch"
		return &EnvelopeError{
			Code:         "session_dial_failed",
			Message:      err.Error(),
			Category:     CategoryConnection,
			Retryable:    true,
			RecoveryHint: `Could not connect to the CDP endpoint. Confirm Excel is running with WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS="--remote-debugging-port=9222", or call addin.launch.`,
			Details:      details,
		}
	}
	return &EnvelopeError{
		Code:      "session_acquire_failed",
		Message:   err.Error(),
		Category:  CategoryConnection,
		Retryable: true,
		Details:   details,
	}
}

// newRequestID returns 16 hex chars of cryptographic randomness, suitable as a
// per-call correlation id. Falls back to a timestamp string only if the OS RNG
// is unavailable — that path should be unreachable in practice.
func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("ts-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
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
func buildRunEnv(sess *session.Session, conn *cdp.Connection, diag *Diagnostics, allowDangerous bool, manifest func() *addin.Manifest) *RunEnv {
	return &RunEnv{
		Diag: diag,
		Conn: func(_ context.Context) (*cdp.Connection, error) {
			return conn, nil
		},
		EnsureEnabled: func(ctx context.Context, cdpSID, domain string) error {
			return sess.EnsureEnabled(ctx, conn, cdpSID, domain)
		},
		AllowDangerous: allowDangerous,
		Attach: func(ctx context.Context, sel TargetSelector) (*AttachedTarget, error) {
			// Empty selector: prefer the sticky default installed by
			// pages.select. Falls through to FirstPageTarget when unset.
			if sel.TargetID == "" && sel.URLPattern == "" && sel.Surface == "" && sel.AddinID == "" {
				if def, ok := sess.DefaultSelection(); ok {
					diag.TargetID = def.Target.TargetID
					diag.CDPSessionID = def.SessionID
					return &AttachedTarget{
						Conn:      conn,
						Target:    def.Target,
						SessionID: def.SessionID,
					}, nil
				}
			}
			key := selectorCacheKey(sel)
			if cached, ok := sess.Selected(sel.TargetID, key); ok {
				diag.TargetID = cached.Target.TargetID
				diag.CDPSessionID = cached.SessionID
				return &AttachedTarget{
					Conn:      conn,
					Target:    cached.Target,
					SessionID: cached.SessionID,
				}, nil
			}
			var m *addin.Manifest
			if manifest != nil {
				m = manifest()
			}
			target, err := ResolveTarget(ctx, conn, sel, m)
			if err != nil {
				return nil, err
			}
			diag.TargetID = target.TargetID
			cdpSID, err := conn.AttachToTarget(ctx, target.TargetID)
			if err != nil {
				return nil, err
			}
			diag.CDPSessionID = cdpSID
			sess.SetSelected(sel.TargetID, key, target, cdpSID)
			return &AttachedTarget{
				Conn:      conn,
				Target:    target,
				SessionID: cdpSID,
			}, nil
		},
		SetDefaultSelection: func(target cdp.TargetInfo, cdpSID string) {
			sess.SetDefaultSelection(target, cdpSID)
		},
		ClearDefaultSelection: func() {
			sess.ClearDefaultSelection()
		},
		Snapshot: func() *session.Snapshot {
			return sess.Snapshot()
		},
		SetSnapshot: func(snap *session.Snapshot) {
			sess.SetSnapshot(snap)
		},
		EventBuf: func(kind session.EventBufKind, cdpSID string, maxBuffer int) *session.EventBuf {
			return sess.EventBuf(kind, cdpSID, maxBuffer)
		},
		MarkEventPumping: func(kind session.EventBufKind, cdpSID string, maxBuffer int) bool {
			return sess.MarkEventPumping(kind, cdpSID, maxBuffer)
		},
	}
}

// selectorCacheKey collapses the non-TargetID portions of a selector into a
// stable string used as the URL-pattern cache key. Surface- and add-in-id
// selectors thus get their own cache slot rather than colliding with a bare
// URL-pattern selector.
func selectorCacheKey(sel TargetSelector) string {
	if sel.URLPattern != "" {
		return sel.URLPattern
	}
	if sel.Surface == "" && sel.AddinID == "" {
		return ""
	}
	return "surface=" + string(sel.Surface) + "|addin=" + sel.AddinID
}
