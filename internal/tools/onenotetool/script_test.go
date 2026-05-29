package onenotetool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRunScript_HappyPath(t *testing.T) {
	env := fakeEnv(t, okOffice(map[string]any{"ok": true}))
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"return 1;"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Ran custom OneNote.run script." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunScript_WithScriptArgs(t *testing.T) {
	// Exercises the len(p.ScriptArgs) > 0 branch.
	env := fakeEnv(t, okOffice(map[string]any{"ok": true}))
	res := runRunScript(context.Background(),
		json.RawMessage(`{"script":"return args.x;","scriptArgs":{"x":42}}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Ran custom OneNote.run script." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunScript_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("GeneralException", "script blew up"))
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"throw 1;"}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "GeneralException" {
		t.Errorf("err=%+v, want office_js/GeneralException", res.Err)
	}
}

func TestRunScript_AttachFailure(t *testing.T) {
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunScript_BadParams(t *testing.T) {
	res := runRunScript(context.Background(), json.RawMessage(`{"script":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunScript_ToolDefinition(t *testing.T) {
	tool := RunScript()
	if tool.Name != "onenote.runScript" {
		t.Errorf("name=%q", tool.Name)
	}
	if tool.Run == nil {
		t.Error("Run is nil")
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
}
