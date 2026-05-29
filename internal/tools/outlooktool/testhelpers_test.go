package outlooktool

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// fakeEnv returns a RunEnv whose Attach hands the tool a real *cdp.Connection
// backed by an in-process CDP server driven by resp. This is the reusable seam
// for happy-path and Office.js-error coverage of every outlook.* run* function.
func fakeEnv(t *testing.T, resp cdptest.Responder) *tools.RunEnv {
	t.Helper()
	srv := cdptest.NewServer(t, resp)
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
		},
	}
}

// fakeEnvWithCache is fakeEnv plus a real file-backed doccache rooted in a
// per-test temp dir. Used to exercise the discover cache-hit / refresh paths.
func fakeEnvWithCache(t *testing.T, resp cdptest.Responder) *tools.RunEnv {
	t.Helper()
	env := fakeEnv(t, resp)
	env.DocCache = newTestStore(t)
	return env
}

// newTestStore opens a doccache rooted in a unique temp file.
func newTestStore(t *testing.T) *doccache.Store {
	t.Helper()
	dir := t.TempDir()
	return doccache.Open(filepath.Join(dir, "doccache.json"), false)
}

// errEnv returns a RunEnv whose Attach always fails — exercises the attach
// failure branch without a server.
func errEnv() *tools.RunEnv {
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return nil, errors.New("no target")
		},
	}
}

// officeReply builds a Responder that returns a successful Office.js payload
// envelope carrying data for every Runtime.evaluate, and an empty result for
// any other CDP command (Target.attachToTarget etc.).
func officeReply(data any) cdptest.Responder {
	return func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOffice(data), nil
		}
		return map[string]any{}, nil
	}
}

// officeErrReply builds a Responder whose Runtime.evaluate signals an Office.js
// error payload.
func officeErrReply(code, msg string, debugInfo any) cdptest.Responder {
	return func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.EvalOfficeErr(code, msg, debugInfo), nil
		}
		return map[string]any{}, nil
	}
}

// protocolExceptionReply builds a Responder whose Runtime.evaluate reports a JS
// exceptionDetails — surfacing as a payload protocol exception.
func protocolExceptionReply(text string) cdptest.Responder {
	return func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return map[string]any{
				"result": map[string]any{"type": "undefined"},
				"exceptionDetails": map[string]any{
					"exceptionId": 1,
					"text":        text,
				},
			}, nil
		}
		return map[string]any{}, nil
	}
}

// cdpErrReply builds a Responder whose Runtime.evaluate fails with a CDP-level
// RemoteError — surfacing as a payload_failed protocol error.
func cdpErrReply(code int, msg string) cdptest.Responder {
	return func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return nil, &cdp.RemoteError{Code: code, Message: msg}
		}
		return map[string]any{}, nil
	}
}
