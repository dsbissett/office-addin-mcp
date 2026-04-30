package tools

import (
	"context"
	"errors"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
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
type Result struct {
	Data any
	Err  *EnvelopeError
}

// OK builds a successful Result.
func OK(data any) Result { return Result{Data: data} }

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

	// Conn returns the CDP connection for this call. In session mode it may
	// be reused across many calls; in one-shot mode it was dialed lazily for
	// this call. Idempotent — repeated calls return the same connection.
	Conn func(ctx context.Context) (*cdp.Connection, error)

	// Attach returns a connection + resolved target + flatten session id.
	// In session mode, hits a per-session selector cache so repeat calls skip
	// Target.getTargets and Target.attachToTarget — surfacing as a sharp drop
	// in Diagnostics.CDPRoundTrips after the first call.
	Attach func(ctx context.Context, sel TargetSelector) (*AttachedTarget, error)
}

// ClassifyCDPErr maps a low-level CDP/transport error to a uniform Result.
// Tools call this from their Run when a cdp.* call returns an error and they
// don't have a more specific code to raise.
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
	return Fail(category, code, err.Error(), retryable)
}
