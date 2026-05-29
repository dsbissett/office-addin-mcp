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
	"github.com/dsbissett/office-addin-mcp/internal/recorder"
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
	// Recorder is the macro recording store. Wired into RunEnv.Recording.
	// Nil when recording is not available.
	Recorder *recorder.Store

	// Recover, when set, attempts to repair a dead CDP connection after
	// session.Acquire fails with session.ErrDialFailed. It relaunches the
	// add-in this server previously started (stop → fresh launch), resets the
	// session pool, and returns the now-live endpoint. It must self-gate:
	// return an error when recovery is impossible or inappropriate (no tracked
	// launch, or the user attached to an external endpoint). Only the
	// persistent MCP server sets this; one-shot/CLI dispatch leaves it nil so
	// no surprise process spawns happen there.
	Recover func(ctx context.Context) (webview2.Config, error)
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

	tool, rawParams, failure := d.lookupAndValidate(req)
	if failure != nil {
		return finalize(diag, start, 0, *failure)
	}

	if tool.NoSession {
		return d.dispatchNoSession(ctx, req, tool, rawParams, &diag, start)
	}
	return d.dispatchWithSession(ctx, req, tool, rawParams, &diag, start, requestID)
}

// lookupAndValidate resolves the tool and normalizes+validates the raw params.
// On failure it returns a populated Result; otherwise the tool and params.
func (d *Dispatcher) lookupAndValidate(req Request) (*Tool, []byte, *Result) {
	tool, ok := d.Registry.Get(req.Tool)
	if !ok {
		return nil, nil, &Result{Err: &EnvelopeError{
			Code:     "unknown_tool",
			Message:  fmt.Sprintf("unknown tool: %s", req.Tool),
			Category: CategoryNotFound,
		}}
	}
	rawParams := req.Params
	if len(rawParams) == 0 {
		rawParams = []byte("{}")
	}
	if err := validateParams(tool.compiled, rawParams); err != nil {
		return nil, nil, &Result{Err: &EnvelopeError{
			Code:     "schema_violation",
			Message:  err.Error(),
			Category: CategoryValidation,
		}}
	}
	return tool, rawParams, nil
}

// dispatchNoSession runs a lifecycle (NoSession) tool with a connection-free
// RunEnv.
func (d *Dispatcher) dispatchNoSession(ctx context.Context, req Request, tool *Tool, rawParams []byte, diag *Diagnostics, start time.Time) Envelope {
	env := &RunEnv{
		Diag:           diag,
		Endpoint:       req.Endpoint,
		AllowDangerous: d.AllowDangerous,
		SetEndpoint:    d.SetEndpoint,
		Manifest:       d.Manifest,
		SetManifest:    d.SetManifest,
		DocCache:       d.DocCache,
		Recorder:       d.Recorder,
		Progress:       req.Progress,
		Log:            req.Log,
	}
	if d.Sessions != nil {
		env.ResetSessions = d.Sessions.DropAll
	}
	if d.Recorder != nil {
		env.Recording = func(tool string, params []byte) error {
			return d.Recorder.Append(tool, params)
		}
	}
	res := runAndEnrich(ctx, tool, req, rawParams, env)
	return finalize(*diag, start, 0, res)
}

// dispatchWithSession acquires a pooled session/connection (with one automatic
// recovery attempt), wires the per-call RunEnv, runs the tool, and finalizes
// with the CDP round-trip delta.
func (d *Dispatcher) dispatchWithSession(ctx context.Context, req Request, tool *Tool, rawParams []byte, diag *Diagnostics, start time.Time, requestID string) Envelope {
	sess := d.Sessions.Get(req.SessionID)
	if d.Ephemeral {
		defer d.Sessions.Drop(req.SessionID)
	}

	conn, release, err := d.acquireWithRecover(ctx, &req, &sess, requestID)
	if err != nil {
		return finalize(*diag, start, 0, Result{Err: classifyAcquireErr(err, req.Endpoint)})
	}
	defer release()

	setEndpointDiag(diag, req.Endpoint)
	rtStart := conn.RoundTrips()

	env := d.buildSessionEnv(req, sess, conn, diag)
	res := runAndEnrich(ctx, tool, req, rawParams, env)
	return finalize(*diag, start, conn.RoundTrips()-rtStart, res)
}

// acquireWithRecover acquires a connection, and on a dial failure attempts one
// automatic stop+relaunch (when d.Recover is set) before re-acquiring against
// the fresh endpoint. req and sess are updated in place when recovery occurs.
func (d *Dispatcher) acquireWithRecover(ctx context.Context, req *Request, sess **session.Session, requestID string) (*cdp.Connection, func(), error) {
	conn, release, err := (*sess).Acquire(ctx, req.Endpoint)
	if err == nil {
		return conn, release, nil
	}
	// Self-healing: a dial failure usually means the Excel this server
	// launched was closed. Try one automatic stop+relaunch, then re-acquire
	// against the fresh endpoint. Recovery resets the session pool, so the
	// retry starts with a clean reconnect budget (recovery launches never
	// consumed it — they use HTTP probes, not session dials).
	if d.Recover == nil || !errors.Is(err, session.ErrDialFailed) {
		return nil, nil, err
	}
	return d.relaunchAndReacquire(ctx, req, sess, requestID, err)
}

// relaunchAndReacquire performs the one-shot recovery: relaunch the add-in and
// re-acquire against the fresh endpoint. On recovery failure it returns the
// original acquire error (acquireErr) so the caller's classification is stable.
func (d *Dispatcher) relaunchAndReacquire(ctx context.Context, req *Request, sess **session.Session, requestID string, acquireErr error) (*cdp.Connection, func(), error) {
	newEP, rerr := d.Recover(ctx)
	if rerr != nil {
		slog.Debug("dispatch.autorecover_skipped", "request_id", requestID, "reason", rerr)
		return nil, nil, acquireErr
	}
	slog.Warn("dispatch.autorecover", "request_id", requestID, "tool", req.Tool, "endpoint", newEP.BrowserURL)
	if req.Log != nil {
		req.Log("warning", "CDP connection was dead; auto-relaunched the add-in and retried.")
	}
	req.Endpoint = newEP
	*sess = d.Sessions.Get(req.SessionID)
	return (*sess).Acquire(ctx, newEP)
}

// buildSessionEnv constructs the per-call RunEnv for the session path, layering
// the dispatcher-scope helpers over the session-bound base.
func (d *Dispatcher) buildSessionEnv(req Request, sess *session.Session, conn *cdp.Connection, diag *Diagnostics) *RunEnv {
	env := buildRunEnv(sess, conn, diag, d.AllowDangerous, d.Manifest, d.Recorder)
	env.Endpoint = req.Endpoint
	env.SetEndpoint = d.SetEndpoint
	env.Manifest = d.Manifest
	env.SetManifest = d.SetManifest
	env.DocCache = d.DocCache
	env.Recorder = d.Recorder
	env.Progress = req.Progress
	env.Log = req.Log
	if d.Sessions != nil {
		env.ResetSessions = d.Sessions.DropAll
	}
	return env
}

// runAndEnrich executes the tool, enriches Office.js errors, and records a
// successful call when recording is active. Shared by both dispatch paths.
func runAndEnrich(ctx context.Context, tool *Tool, req Request, rawParams []byte, env *RunEnv) Result {
	res := tool.Run(ctx, rawParams, env)
	if res.Err != nil && res.Err.Category == CategoryOfficeJS {
		classifyOfficeJSErr(ctx, env, req.Tool, rawParams, res.Err)
	}
	// Record successful tool calls when recording is active.
	if res.Err == nil && env.Recording != nil {
		_ = env.Recording(req.Tool, rawParams)
	}
	return res
}

// setEndpointDiag stamps the endpoint diagnostic from the call's endpoint
// config, preferring the WS endpoint over the browser URL.
func setEndpointDiag(diag *Diagnostics, ep webview2.Config) {
	if ep.WSEndpoint != "" {
		diag.Endpoint = ep.WSEndpoint
	} else if ep.BrowserURL != "" {
		diag.Endpoint = ep.BrowserURL
	}
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
	details := map[string]any{"probedEndpoint": probedEndpoint(ep)}

	switch {
	case errors.Is(err, session.ErrReconnectBudgetExhausted):
		details["recoverableViaTool"] = "addin.ensureRunning"
		return &EnvelopeError{
			Code:         "session_reconnect_budget_exhausted",
			Message:      err.Error(),
			Category:     CategoryConnection,
			Retryable:    false,
			RecoveryHint: "Reconnect budget (3 attempts per 60s) is exhausted. Excel may not be running with --remote-debugging-port=9222 — call addin.ensureRunning, or wait 60 seconds and retry.",
			Details:      details,
		}
	case errors.Is(err, context.DeadlineExceeded):
		return &EnvelopeError{
			Code:         "session_acquire_timeout",
			Message:      err.Error(),
			Category:     CategoryTimeout,
			Retryable:    true,
			RecoveryHint: "Tool call timed out before the CDP connection was ready. Retry with a longer ctx deadline, or call addin.ensureRunning if Excel is not running.",
			Details:      details,
		}
	case errors.Is(err, session.ErrDialFailed):
		details["recoverableViaTool"] = "addin.ensureRunning"
		return &EnvelopeError{
			Code:         "session_dial_failed",
			Message:      err.Error(),
			Category:     CategoryConnection,
			Retryable:    true,
			RecoveryHint: `Could not connect to the CDP endpoint. Confirm Excel is running with WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS="--remote-debugging-port=9222", or call addin.ensureRunning.`,
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

// probedEndpoint returns the endpoint string we tried to reach, preferring the
// WS endpoint, then the browser URL, then the conventional default port.
func probedEndpoint(ep webview2.Config) string {
	if ep.WSEndpoint != "" {
		return ep.WSEndpoint
	}
	if ep.BrowserURL != "" {
		return ep.BrowserURL
	}
	return "http://127.0.0.1:9222"
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
func buildRunEnv(sess *session.Session, conn *cdp.Connection, diag *Diagnostics, allowDangerous bool, manifest func() *addin.Manifest, rec *recorder.Store) *RunEnv {
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
			return attachTarget(ctx, sess, conn, diag, manifest, sel)
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
		Recording: func(tool string, params []byte) error {
			if rec == nil {
				return nil
			}
			return rec.Append(tool, params)
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

// attachTarget resolves and attaches the target for a selector. It prefers the
// sticky default for an empty selector, then the per-session selector cache,
// finally resolving live and attaching (caching the result). Diagnostics are
// stamped with the chosen target/CDP session in every branch.
func attachTarget(ctx context.Context, sess *session.Session, conn *cdp.Connection, diag *Diagnostics, manifest func() *addin.Manifest, sel TargetSelector) (*AttachedTarget, error) {
	if isEmptySelector(sel) {
		if def, ok := sess.DefaultSelection(); ok {
			return attachedFromCache(conn, diag, def.Target, def.SessionID), nil
		}
	}
	key := selectorCacheKey(sel)
	if cached, ok := sess.Selected(sel.TargetID, key); ok {
		return attachedFromCache(conn, diag, cached.Target, cached.SessionID), nil
	}
	return resolveAndAttach(ctx, sess, conn, diag, manifest, sel, key)
}

// isEmptySelector reports whether no selection criteria were supplied.
func isEmptySelector(sel TargetSelector) bool {
	return sel.TargetID == "" && sel.URLPattern == "" && sel.Surface == "" && sel.AddinID == ""
}

// attachedFromCache stamps diagnostics and builds an AttachedTarget for a
// target/CDP session already known to the session (default or cache hit).
func attachedFromCache(conn *cdp.Connection, diag *Diagnostics, target cdp.TargetInfo, cdpSID string) *AttachedTarget {
	diag.TargetID = target.TargetID
	diag.CDPSessionID = cdpSID
	return &AttachedTarget{Conn: conn, Target: target, SessionID: cdpSID}
}

// resolveAndAttach resolves the selector against the live connection, attaches
// to the chosen target, caches the result, and stamps diagnostics.
func resolveAndAttach(ctx context.Context, sess *session.Session, conn *cdp.Connection, diag *Diagnostics, manifest func() *addin.Manifest, sel TargetSelector, key string) (*AttachedTarget, error) {
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
	return &AttachedTarget{Conn: conn, Target: target, SessionID: cdpSID}, nil
}
