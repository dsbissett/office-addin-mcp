package powerpointtool

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
// for happy-path and Office.js-error coverage of every powerpoint.* run*
// function.
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

// fakeEnvWithCache is fakeEnv plus a real on-disk doccache.Store rooted in a
// per-test temp dir. Discover tests need a live cache to exercise the
// cache-hit / refresh branches.
func fakeEnvWithCache(t *testing.T, resp cdptest.Responder) (*tools.RunEnv, *doccache.Store) {
	t.Helper()
	store := doccache.Open(filepath.Join(t.TempDir(), "doccache.json"), false)
	env := fakeEnv(t, resp)
	env.DocCache = store
	return env, store
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

// office wraps the common happy-path responder that returns a successful
// Office.js payload envelope carrying data.
func officeOK(data any) cdptest.Responder {
	return func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOffice(data), nil
	}
}

// officeErr wraps the common responder that returns an Office.js error
// envelope.
func officeErr(code, msg string) cdptest.Responder {
	return func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOfficeErr(code, msg, nil), nil
	}
}
