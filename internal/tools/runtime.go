package tools

import (
	"context"
	"errors"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// Request is the dispatcher's input.
type Request struct {
	Tool      string
	Params    []byte // raw JSON bytes for the params object
	Endpoint  webview2.Config
	SessionID string // Phase 5 user session id; empty resolves to "default"
}

// Result is what a tool's Run function returns. Exactly one of Data/Err is set.
//
// Summary is an optional one-line human-readable message. When non-empty the
// MCP adapter prepends it as a TextContent block ahead of the JSON payload, so
// chat clients display friendly text in the tool's OUT bubble while agents
// still parse the structured Data block. Leave empty for tools where the JSON
// payload is already self-explanatory or where there is nothing meaningful to
// announce. Set on both success and failure paths.
type Result struct {
	Data    any
	Err     *EnvelopeError
	Summary string
}

// OK builds a successful Result.
func OK(data any) Result { return Result{Data: data} }

// OKWithSummary builds a successful Result with a human-readable summary line.
func OKWithSummary(summary string, data any) Result {
	return Result{Data: data, Summary: summary}
}

// Fail builds a failure Result.
func Fail(category, code, msg string, retryable bool) Result {
	return Result{Err: &EnvelopeError{
		Code:      code,
		Message:   msg,
		Category:  category,
		Retryable: retryable,
	}}
}

// FailWithDetails attaches details to a Fail.
func FailWithDetails(category, code, msg string, retryable bool, details map[string]any) Result {
	return Result{Err: &EnvelopeError{
		Code:      code,
		Message:   msg,
		Category:  category,
		Retryable: retryable,
		Details:   details,
	}}
}

// AttachedTarget bundles a connection + resolved target + CDP flatten session.
// Tools MUST NOT close the connection — the dispatcher owns its lifetime.
type AttachedTarget struct {
	Conn      *cdp.Connection
	Target    cdp.TargetInfo
	SessionID string // CDP flatten session id (Target.attachToTarget result)
}

// RunEnv is the runtime context handed to a tool's Run function. The
// dispatcher constructs it per call; helpers close over either an ephemeral
// connection (one-shot mode) or a session-pooled one (daemon mode). Tools
// must NOT manage connection lifetime — call Conn or Attach and use the
// result for the duration of Run.
type RunEnv struct {
	Diag *Diagnostics

	// Endpoint is the CDP endpoint config the dispatcher resolved for this
	// call. Always populated, even on NoSession lifecycle tools, so tools
	// like addin.status can probe the configured endpoint without the
	// caller having to thread it in as a parameter. Read-only — to mutate
	// the server's default endpoint use SetEndpoint.
	Endpoint webview2.Config

	// Conn returns the CDP connection for this call. In session mode it may
	// be reused across many calls; in one-shot mode it was dialed lazily for
	// this call. Idempotent — repeated calls return the same connection.
	Conn func(ctx context.Context) (*cdp.Connection, error)

	// Attach returns a connection + resolved target + flatten session id.
	// In session mode, hits a per-session selector cache so repeat calls skip
	// Target.getTargets and Target.attachToTarget — surfacing as a sharp drop
	// in Diagnostics.CDPRoundTrips after the first call.
	Attach func(ctx context.Context, sel TargetSelector) (*AttachedTarget, error)

	// EnsureEnabled issues "<domain>.enable" once per (cdpSessionID, domain)
	// pair for the current session. Generated CDP tools call this before
	// their first command on any auto-enable domain (Page, Runtime, DOM,
	// CSS, Network, Fetch, …). Re-runs only after the underlying CDP
	// connection drops, which clears the bookkeeping.
	EnsureEnabled func(ctx context.Context, cdpSessionID, domain string) error

	// AllowDangerous gates methods marked dangerous in the manifest
	// (Browser.crash, Runtime.terminateExecution, etc.). Set from the
	// process-wide --allow-dangerous-cdp flag / OAMCP_ALLOW_DANGEROUS_CDP
	// env var. When false, generated dangerous tools refuse with
	// dangerous_disabled.
	AllowDangerous bool

	// SetEndpoint overrides the server's default CDP endpoint for subsequent
	// tool calls. addin.launch uses this so an agent never needs to pass
	// --browser-url after sideloading Excel. Nil-safe; tools should call
	// only when present.
	SetEndpoint func(webview2.Config)

	// Manifest returns the parsed manifest of the active add-in launch, or
	// nil if no manifest is loaded. Tools that classify CDP targets by
	// surface (taskpane / dialog / cf-runtime) consult this. Always nil-safe.
	Manifest func() *addin.Manifest

	// SetManifest stores a parsed manifest at server scope. addin.launch
	// invokes this after a successful sideload so subsequent surface-based
	// selectors can resolve. Nil-safe.
	SetManifest func(*addin.Manifest)

	// SetDefaultSelection records the page chosen by pages.select. When
	// installed, subsequent Attach calls with an empty selector return this
	// page rather than FirstPageTarget. Nil in lifecycle (NoSession) tools.
	SetDefaultSelection func(target cdp.TargetInfo, cdpSessionID string)

	// ClearDefaultSelection drops the sticky default. Used by pages.close
	// when the closed target is the active default. Nil in NoSession tools.
	ClearDefaultSelection func()

	// Snapshot returns the most recent page.snapshot output, or nil. Used by
	// interaction tools (page.click, page.fill, …) to resolve UIDs to
	// backendNodeIds. Nil in NoSession tools.
	Snapshot func() *session.Snapshot

	// SetSnapshot installs a fresh snapshot, clearing the previous one. Used
	// by page.snapshot. Nil in NoSession tools.
	SetSnapshot func(*session.Snapshot)

	// EventBuf returns the get-or-create event ring for (kind, cdpSessionID),
	// honoring the supplied maxBuffer on first access (and resizing on
	// subsequent calls). Used by page.consoleLog / page.networkLog. Nil in
	// NoSession tools.
	EventBuf func(kind session.EventBufKind, cdpSessionID string, maxBuffer int) *session.EventBuf

	// MarkEventPumping atomically checks whether the (kind, cdpSessionID)
	// pump goroutine is already running and reserves the slot when it isn't.
	// Returns true exactly once per session per (kind, cdpSessionID) — the
	// caller is then responsible for spawning the pump. Nil in NoSession tools.
	MarkEventPumping func(kind session.EventBufKind, cdpSessionID string, maxBuffer int) bool

	// DocCache is the persistent document discovery cache shared across
	// sessions and processes. nil-safe via the package's pointer-receiver
	// methods — discover/query tools can call DocCache.Get/Put unconditionally
	// even when --no-doccache is set (Open returns a disabled store).
	DocCache *doccache.Store
}

// ClassifyCDPErr maps a low-level CDP/transport error to a uniform Result.
// Tools call this from their Run when a cdp.* call returns an error and they
// don't have a more specific code to raise.
//
// When err wraps a *cdp.RemoteError, its structured {code, message, data}
// fields are surfaced in Details["cdpError"] so an AI caller can branch on the
// CDP-level code instead of regexing the Message.
func ClassifyCDPErr(code string, err error) Result {
	category := CategoryProtocol
	retryable := false
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		category = CategoryTimeout
		code = "timeout"
		retryable = true
	case errors.Is(err, context.Canceled):
		category = CategoryInternal
		code = "canceled"
	case errors.Is(err, cdp.ErrClosed):
		category = CategoryConnection
		retryable = true
	}
	var remoteErr *cdp.RemoteError
	if errors.As(err, &remoteErr) {
		details := map[string]any{
			"cdpError": map[string]any{
				"code":    remoteErr.Code,
				"message": remoteErr.Message,
				"data":    remoteErr.Data,
			},
		}
		return FailWithDetails(category, code, err.Error(), retryable, details)
	}
	return Fail(category, code, err.Error(), retryable)
}
