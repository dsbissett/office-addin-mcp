package tools

import (
	"context"
	"errors"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// Request is the dispatcher's input.
type Request struct {
	Tool     string
	Params   []byte // raw JSON bytes for the params object
	Endpoint webview2.Config
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

// RunEnv is what each tool's Run sees. It is constructed by Dispatch and is
// not safe for use after Run returns.
type RunEnv struct {
	Diag *Diagnostics

	// OpenConn dials a fresh CDP connection in one-shot mode. The tool owns
	// the lifetime — it must Close. Phase 5 will replace this with a
	// session-pooled handle.
	OpenConn func(ctx context.Context) (*cdp.Connection, error)
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
