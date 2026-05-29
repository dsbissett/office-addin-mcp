package exceltool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// fakeEnv returns a RunEnv whose Attach hands the tool a real *cdp.Connection
// backed by an in-process CDP server driven by resp. This is the reusable seam
// for happy-path and Office.js-error coverage of every excel.* run* function.
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

func TestRunWorksheetInfo_HappyPath(t *testing.T) {
	env := fakeEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOffice(map[string]any{"name": "Sheet1", "usedRangeAddress": "A1:C9"}), nil
	})
	res := runWorksheetInfo(context.Background(), json.RawMessage(`{"sheet":"Sheet1"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Worksheet Sheet1: used range A1:C9." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunWorksheetInfo_OfficeError(t *testing.T) {
	env := fakeEnv(t, func(string, json.RawMessage) (any, *cdp.RemoteError) {
		return cdptest.EvalOfficeErr("ItemNotFound", "Worksheet not found", nil), nil
	})
	res := runWorksheetInfo(context.Background(), json.RawMessage(`{"sheet":"Nope"}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "ItemNotFound" {
		t.Errorf("err=%+v, want office_js/ItemNotFound", res.Err)
	}
}

func TestRunWorksheetInfo_AttachFailure(t *testing.T) {
	res := runWorksheetInfo(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunWorksheetInfo_BadParams(t *testing.T) {
	res := runWorksheetInfo(context.Background(), json.RawMessage(`{"sheet":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}
