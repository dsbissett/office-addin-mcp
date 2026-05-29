package onenotetool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestAppendToPage_HTMLOnly_TitleSummary(t *testing.T) {
	env := fakeEnv(t, okOffice(map[string]any{"title": "Meeting Notes"}))
	res := runAppendToPage(context.Background(),
		json.RawMessage(`{"html":"<p>hi</p>"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != `Appended content to "Meeting Notes".` {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestAppendToPage_BulletsOnly_FallbackTitle(t *testing.T) {
	// No title in the payload result => summary falls back to "page".
	env := fakeEnv(t, okOffice(map[string]any{}))
	res := runAppendToPage(context.Background(),
		json.RawMessage(`{"bullets":["a","b"]}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != `Appended content to "page".` {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestAppendToPage_HTMLAndBulletsAndPageID(t *testing.T) {
	// Exercises all three arg-building branches: pageId, html, bullets.
	env := fakeEnv(t, okOffice(map[string]any{"title": "Page X"}))
	res := runAppendToPage(context.Background(),
		json.RawMessage(`{"pageId":"p1","html":"<p>x</p>","bullets":["one"]}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != `Appended content to "Page X".` {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestAppendToPage_NothingToAppend(t *testing.T) {
	// Neither html nor bullets => validation failure before any attach.
	res := runAppendToPage(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "nothing_to_append" {
		t.Fatalf("want nothing_to_append, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestAppendToPage_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("ItemNotFound", "page not found"))
	res := runAppendToPage(context.Background(),
		json.RawMessage(`{"html":"<p>x</p>"}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "ItemNotFound" {
		t.Errorf("err=%+v, want office_js/ItemNotFound", res.Err)
	}
}

func TestAppendToPage_AttachFailure(t *testing.T) {
	res := runAppendToPage(context.Background(),
		json.RawMessage(`{"html":"<p>x</p>"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestAppendToPage_BadParams(t *testing.T) {
	res := runAppendToPage(context.Background(),
		json.RawMessage(`{"html":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestAppendToPage_ToolDefinition(t *testing.T) {
	tool := AppendToPage()
	if tool.Name != "onenote.appendToPage" {
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
