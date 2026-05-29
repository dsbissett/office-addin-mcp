package onenotetool

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
// for happy-path and Office.js-error coverage of every onenote.* run function.
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

// fakeEnvWithCache is fakeEnv plus a real file-backed doccache, used by the
// discover tests which consult env.DocCache.
func fakeEnvWithCache(t *testing.T, resp cdptest.Responder) *tools.RunEnv {
	t.Helper()
	env := fakeEnv(t, resp)
	env.DocCache = openTestStore(t)
	return env
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

// openTestStore returns a real doccache.Store rooted at a unique temp file.
func openTestStore(t *testing.T) *doccache.Store {
	t.Helper()
	dir := t.TempDir()
	return doccache.Open(filepath.Join(dir, "doccache.json"), false)
}

// okOffice is a Responder convenience that always replies with the Office.js
// success envelope wrapping data.
func okOffice(data any) cdptest.Responder {
	return func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOffice(data), nil
	}
}

// officeErr is a Responder convenience that always replies with an Office.js
// error envelope.
func officeErr(code, msg string) cdptest.Responder {
	return func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOfficeErr(code, msg, nil), nil
	}
}
