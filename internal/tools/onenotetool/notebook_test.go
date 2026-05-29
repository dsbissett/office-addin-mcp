package onenotetool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// ---------- onenote.readNotebooks ----------

func TestReadNotebooks_HappyPath(t *testing.T) {
	env := fakeEnv(t, okOffice(map[string]any{
		"notebooks": []any{map[string]any{"name": "A"}, map[string]any{"name": "B"}},
	}))
	res := runReadNotebooks(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Listed 2 notebook(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestReadNotebooks_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("GeneralException", "boom"))
	res := runReadNotebooks(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js error, got %+v", res.Err)
	}
}

func TestReadNotebooks_AttachFailure(t *testing.T) {
	res := runReadNotebooks(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestReadNotebooks_BadParams(t *testing.T) {
	res := runReadNotebooks(context.Background(), json.RawMessage(`{"targetId":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestReadNotebooks_ToolDefinition(t *testing.T) {
	tool := ReadNotebooks()
	if tool.Name != "onenote.readNotebooks" || tool.Run == nil {
		t.Errorf("def=%+v", tool)
	}
}

// ---------- onenote.readSections ----------

func TestReadSections_WithNotebookName(t *testing.T) {
	env := fakeEnv(t, okOffice(map[string]any{
		"notebookName": "Work",
		"sections":     []any{map[string]any{"name": "S1"}},
	}))
	res := runReadSections(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Listed 1 section(s) in Work." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestReadSections_WithoutNotebookName(t *testing.T) {
	env := fakeEnv(t, okOffice(map[string]any{
		"sections": []any{map[string]any{"name": "S1"}, map[string]any{"name": "S2"}},
	}))
	res := runReadSections(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Listed 2 section(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestReadSections_AttachFailure(t *testing.T) {
	res := runReadSections(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestReadSections_BadParams(t *testing.T) {
	res := runReadSections(context.Background(), json.RawMessage(`{"urlPattern":1}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestReadSections_ToolDefinition(t *testing.T) {
	tool := ReadSections()
	if tool.Name != "onenote.readSections" || tool.Run == nil {
		t.Errorf("def=%+v", tool)
	}
}

// ---------- onenote.readPages ----------

func TestReadPages_WithSectionName(t *testing.T) {
	env := fakeEnv(t, okOffice(map[string]any{
		"sectionName": "Notes",
		"pages":       []any{map[string]any{"title": "P1"}},
	}))
	res := runReadPages(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Listed 1 page(s) in Notes." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestReadPages_WithoutSectionName(t *testing.T) {
	env := fakeEnv(t, okOffice(map[string]any{"pages": []any{}}))
	res := runReadPages(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Listed 0 page(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestReadPages_AttachFailure(t *testing.T) {
	res := runReadPages(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestReadPages_BadParams(t *testing.T) {
	res := runReadPages(context.Background(), json.RawMessage(`{"targetId":[]}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestReadPages_ToolDefinition(t *testing.T) {
	tool := ReadPages()
	if tool.Name != "onenote.readPages" || tool.Run == nil {
		t.Errorf("def=%+v", tool)
	}
}

// ---------- onenote.readPage ----------

func TestReadPage_WithTitle(t *testing.T) {
	env := fakeEnv(t, okOffice(map[string]any{
		"title":    "My Page",
		"contents": []any{map[string]any{"id": "c1", "type": "Outline"}},
	}))
	res := runReadPage(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != `Read page "My Page" (1 content item(s)).` {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestReadPage_WithoutTitle(t *testing.T) {
	env := fakeEnv(t, okOffice(map[string]any{"contents": []any{}}))
	res := runReadPage(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read page (0 content item(s))." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestReadPage_AttachFailure(t *testing.T) {
	res := runReadPage(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestReadPage_BadParams(t *testing.T) {
	res := runReadPage(context.Background(), json.RawMessage(`{"targetId":{}}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestReadPage_ToolDefinition(t *testing.T) {
	tool := ReadPage()
	if tool.Name != "onenote.readPage" || tool.Run == nil {
		t.Errorf("def=%+v", tool)
	}
}

// ---------- onenote.addPage ----------

func TestAddPage_HappyPath(t *testing.T) {
	env := fakeEnv(t, okOffice(map[string]any{"id": "new-page"}))
	res := runAddPage(context.Background(), json.RawMessage(`{"title":"Fresh"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Added page: Fresh" {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestAddPage_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("ItemNotFound", "no section"))
	res := runAddPage(context.Background(), json.RawMessage(`{"title":"X"}`), env)
	if res.Err == nil || res.Err.Code != "ItemNotFound" {
		t.Fatalf("want ItemNotFound, got %+v", res.Err)
	}
}

func TestAddPage_AttachFailure(t *testing.T) {
	res := runAddPage(context.Background(), json.RawMessage(`{"title":"X"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestAddPage_BadParams(t *testing.T) {
	res := runAddPage(context.Background(), json.RawMessage(`{"title":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestAddPage_ToolDefinition(t *testing.T) {
	tool := AddPage()
	if tool.Name != "onenote.addPage" || tool.Run == nil {
		t.Errorf("def=%+v", tool)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
}
