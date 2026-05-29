package powerpointtool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRunRunScript_HappyPath(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{"ok": true}))
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"return 1;"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Ran custom PowerPoint.run script." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunRunScript_WithScriptArgs(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{"ok": true}))
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"return args.x;","scriptArgs":{"x":7}}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Ran custom PowerPoint.run script." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunRunScript_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("GeneralException", "boom"))
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"throw new Error();"}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "GeneralException" {
		t.Errorf("err=%+v, want office_js/GeneralException", res.Err)
	}
}

func TestRunRunScript_AttachFailure(t *testing.T) {
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunRunScript_BadParams(t *testing.T) {
	res := runRunScript(context.Background(), json.RawMessage(`{"script":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}
