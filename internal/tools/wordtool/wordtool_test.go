package wordtool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// ---------- word.applyEdits ----------

func TestRunApplyEdits_HappyPath(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{
		"edits": []any{
			map[string]any{"replaced": float64(2)},
			map[string]any{"replaced": float64(3)},
		},
	}))
	res := runApplyEdits(context.Background(),
		json.RawMessage(`{"edits":[{"find":"foo","replace":"bar"},{"find":"baz","replace":""}]}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Replaced 5 occurrence(s) across 2 edit(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

// HappyPath where the payload data is not a map / has no edits array — the
// summary defaults to 0 replacements but still counts the requested edits.
func TestRunApplyEdits_HappyPath_NoReplacedCount(t *testing.T) {
	env := fakeEnv(t, okResponder("not-a-map"))
	res := runApplyEdits(context.Background(),
		json.RawMessage(`{"edits":[{"find":"foo"}]}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Replaced 0 occurrence(s) across 1 edit(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunApplyEdits_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrResponder("InvalidArgument", "bad search", nil))
	res := runApplyEdits(context.Background(),
		json.RawMessage(`{"edits":[{"find":"foo"}]}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "InvalidArgument" {
		t.Errorf("err=%+v, want office_js/InvalidArgument", res.Err)
	}
}

func TestRunApplyEdits_AttachFailure(t *testing.T) {
	res := runApplyEdits(context.Background(),
		json.RawMessage(`{"edits":[{"find":"foo"}]}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunApplyEdits_BadParams(t *testing.T) {
	res := runApplyEdits(context.Background(), json.RawMessage(`{"edits":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunApplyEdits_NoEdits(t *testing.T) {
	res := runApplyEdits(context.Background(), json.RawMessage(`{"edits":[]}`), errEnv())
	if res.Err == nil || res.Err.Code != "no_edits" {
		t.Fatalf("want no_edits, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category=%q, want validation", res.Err.Category)
	}
}

// ---------- word.runScript ----------

func TestRunRunScript_HappyPath(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{"ok": true}))
	res := runRunScript(context.Background(),
		json.RawMessage(`{"script":"return 1;","scriptArgs":{"x":1}}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Ran custom Word.run script." {
		t.Errorf("summary=%q", res.Summary)
	}
}

// HappyPath with no scriptArgs exercises the len(p.ScriptArgs)==0 branch.
func TestRunRunScript_HappyPath_NoArgs(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{"ok": true}))
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"return 1;"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Ran custom Word.run script." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunRunScript_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrResponder("GeneralException", "script threw", nil))
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"boom"}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "GeneralException" {
		t.Fatalf("want office_js/GeneralException, got %+v", res.Err)
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

// ---------- word.discover ----------

func TestRunDiscover_HappyPath_Refresh(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{
		"filePath":    `C:\docs\report.docx`,
		"fingerprint": "fp-1",
		"title":       "Report",
		"wordCount":   float64(1200),
	}))
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "Word discovery refreshed") {
		t.Errorf("summary=%q, want refreshed", res.Summary)
	}
	m, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type %T, want map", res.Data)
	}
	if m["cached"] != false {
		t.Errorf("cached=%v, want false", m["cached"])
	}
	if m["fingerprint"] != "fp-1" {
		t.Errorf("fingerprint=%v, want fp-1", m["fingerprint"])
	}
}

// CacheHit: pre-seed the same store the env uses, then discover with a matching
// fingerprint returns the cached snapshot.
func TestRunDiscover_CacheHit(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{
		"filePath":    `C:\docs\cached.docx`,
		"fingerprint": "fp-cached",
		"title":       "Live",
	}))
	// Seed the cache with a fingerprint that matches what the payload returns.
	if err := env.DocCache.Put(doccache.Entry{
		Host:        "word",
		FilePath:    `C:\docs\cached.docx`,
		Fingerprint: "fp-cached",
		Data:        json.RawMessage(`{"title":"FromCache"}`),
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "cache hit") {
		t.Errorf("summary=%q, want cache hit", res.Summary)
	}
	m, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("data type %T, want map", res.Data)
	}
	if m["cached"] != true {
		t.Errorf("cached=%v, want true", m["cached"])
	}
	if m["title"] != "FromCache" {
		t.Errorf("title=%v, want FromCache (cached value)", m["title"])
	}
}

// Force=true bypasses a matching cache entry and refreshes.
func TestRunDiscover_Force(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{
		"filePath":    `C:\docs\force.docx`,
		"fingerprint": "fp-force",
		"title":       "Live",
	}))
	if err := env.DocCache.Put(doccache.Entry{
		Host:        "word",
		FilePath:    `C:\docs\force.docx`,
		Fingerprint: "fp-force",
		Data:        json.RawMessage(`{"title":"FromCache"}`),
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	res := runDiscover(context.Background(), json.RawMessage(`{"force":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "refreshed") {
		t.Errorf("summary=%q, want refreshed (force bypasses cache)", res.Summary)
	}
	m := res.Data.(map[string]any)
	if m["title"] != "Live" {
		t.Errorf("title=%v, want Live", m["title"])
	}
}

func TestRunDiscover_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrResponder("ItemNotFound", "no document", nil))
	res := runDiscover(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "ItemNotFound" {
		t.Fatalf("want office_js/ItemNotFound, got %+v", res.Err)
	}
}

func TestRunDiscover_AttachFailure(t *testing.T) {
	res := runDiscover(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunDiscover_BadParams(t *testing.T) {
	res := runDiscover(context.Background(), json.RawMessage(`{"force":"yes"}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- word.readBody ----------

func TestRunReadBody_HappyPath(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{"text": "Hello, world."}))
	res := runReadBody(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read 13 characters from document body." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadBody_Empty(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{"text": ""}))
	res := runReadBody(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Document body is empty." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadBody_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrResponder("AccessDenied", "locked", nil))
	res := runReadBody(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "AccessDenied" {
		t.Fatalf("want office_js/AccessDenied, got %+v", res.Err)
	}
}

func TestRunReadBody_AttachFailure(t *testing.T) {
	res := runReadBody(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunReadBody_BadParams(t *testing.T) {
	res := runReadBody(context.Background(), json.RawMessage(`{"targetId":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- word.writeBody ----------

func TestRunWriteBody_HappyPath_DefaultLocation(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{}))
	res := runWriteBody(context.Background(), json.RawMessage(`{"text":"abcd"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Wrote 4 characters to document body (Replace)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunWriteBody_HappyPath_ExplicitLocation(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{}))
	res := runWriteBody(context.Background(), json.RawMessage(`{"text":"hi","location":"End"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Wrote 2 characters to document body (End)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunWriteBody_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrResponder("InvalidArgument", "bad", nil))
	res := runWriteBody(context.Background(), json.RawMessage(`{"text":"x"}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "InvalidArgument" {
		t.Fatalf("want office_js/InvalidArgument, got %+v", res.Err)
	}
}

func TestRunWriteBody_AttachFailure(t *testing.T) {
	res := runWriteBody(context.Background(), json.RawMessage(`{"text":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunWriteBody_BadParams(t *testing.T) {
	res := runWriteBody(context.Background(), json.RawMessage(`{"text":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- word.readParagraphs ----------

func TestRunReadParagraphs_HappyPath(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{
		"paragraphs": []any{
			map[string]any{"text": "One", "style": "Normal"},
			map[string]any{"text": "Two", "style": "Heading 1"},
		},
	}))
	res := runReadParagraphs(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read 2 paragraph(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

// Data without a paragraphs array exercises arrayLen's zero-return branch.
func TestRunReadParagraphs_NoArray(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{"other": "x"}))
	res := runReadParagraphs(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read 0 paragraph(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadParagraphs_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrResponder("GeneralException", "boom", nil))
	res := runReadParagraphs(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js, got %+v", res.Err)
	}
}

func TestRunReadParagraphs_AttachFailure(t *testing.T) {
	res := runReadParagraphs(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunReadParagraphs_BadParams(t *testing.T) {
	res := runReadParagraphs(context.Background(), json.RawMessage(`{"urlPattern":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- word.insertParagraph ----------

func TestRunInsertParagraph_HappyPath_DefaultLocationWithStyle(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{"style": "Heading 2"}))
	res := runInsertParagraph(context.Background(), json.RawMessage(`{"text":"hi"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Inserted paragraph at End (style=Heading 2)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunInsertParagraph_HappyPath_ExplicitLocationNoStyle(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{}))
	res := runInsertParagraph(context.Background(), json.RawMessage(`{"text":"hi","location":"Start"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Inserted paragraph at Start." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunInsertParagraph_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrResponder("InvalidArgument", "bad", nil))
	res := runInsertParagraph(context.Background(), json.RawMessage(`{"text":"x"}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js, got %+v", res.Err)
	}
}

func TestRunInsertParagraph_AttachFailure(t *testing.T) {
	res := runInsertParagraph(context.Background(), json.RawMessage(`{"text":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunInsertParagraph_BadParams(t *testing.T) {
	res := runInsertParagraph(context.Background(), json.RawMessage(`{"text":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- word.readSelection ----------

func TestRunReadSelection_HappyPath(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{"text": "selected"}))
	res := runReadSelection(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read selection (8 characters)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadSelection_Empty(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{"text": ""}))
	res := runReadSelection(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "No active selection." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadSelection_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrResponder("GeneralException", "boom", nil))
	res := runReadSelection(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js, got %+v", res.Err)
	}
}

func TestRunReadSelection_AttachFailure(t *testing.T) {
	res := runReadSelection(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunReadSelection_BadParams(t *testing.T) {
	res := runReadSelection(context.Background(), json.RawMessage(`{"targetId":true}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- word.searchText ----------

func TestRunSearchText_HappyPath_QueryOnly(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{
		"matches": []any{map[string]any{"text": "foo"}},
	}))
	res := runSearchText(context.Background(), json.RawMessage(`{"query":"foo"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != `Found 1 match(es) for "foo".` {
		t.Errorf("summary=%q", res.Summary)
	}
}

// Exercises both the matchCase and matchWholeWord optional-flag branches.
func TestRunSearchText_HappyPath_WithFlags(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{
		"matches": []any{
			map[string]any{"text": "Foo"},
			map[string]any{"text": "Foo"},
		},
	}))
	res := runSearchText(context.Background(),
		json.RawMessage(`{"query":"Foo","matchCase":true,"matchWholeWord":false}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != `Found 2 match(es) for "Foo".` {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunSearchText_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrResponder("InvalidArgument", "bad search", nil))
	res := runSearchText(context.Background(), json.RawMessage(`{"query":"x"}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js, got %+v", res.Err)
	}
}

func TestRunSearchText_AttachFailure(t *testing.T) {
	res := runSearchText(context.Background(), json.RawMessage(`{"query":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunSearchText_BadParams(t *testing.T) {
	res := runSearchText(context.Background(), json.RawMessage(`{"query":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- word.readProperties ----------

func TestRunReadProperties_HappyPath_WithTitle(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{"title": "My Doc", "author": "Jane"}))
	res := runReadProperties(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read properties for My Doc." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadProperties_HappyPath_NoTitle(t *testing.T) {
	env := fakeEnv(t, okResponder(map[string]any{"author": "Jane"}))
	res := runReadProperties(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read document properties." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadProperties_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrResponder("AccessDenied", "locked", nil))
	res := runReadProperties(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js, got %+v", res.Err)
	}
}

func TestRunReadProperties_AttachFailure(t *testing.T) {
	res := runReadProperties(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunReadProperties_BadParams(t *testing.T) {
	res := runReadProperties(context.Background(), json.RawMessage(`{"urlPattern":false}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// ---------- tool constructors ----------

// TestConstructors covers the unregistered tool constructors (ReadBody,
// WriteBody, ReadParagraphs, InsertParagraph, ReadSelection, SearchText,
// ReadProperties) so their metadata + non-nil Run are exercised. RunScript,
// ApplyEdits and Discover are already covered via Register in register_test.go.
func TestConstructors(t *testing.T) {
	cases := []struct {
		name string
		tool tools.Tool
	}{
		{"word.readBody", ReadBody()},
		{"word.writeBody", WriteBody()},
		{"word.readParagraphs", ReadParagraphs()},
		{"word.insertParagraph", InsertParagraph()},
		{"word.readSelection", ReadSelection()},
		{"word.searchText", SearchText()},
		{"word.readProperties", ReadProperties()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.tool.Name != tc.name {
				t.Errorf("Name=%q, want %q", tc.tool.Name, tc.name)
			}
			if tc.tool.Run == nil {
				t.Error("Run is nil")
			}
			if len(tc.tool.Schema) == 0 {
				t.Error("Schema is empty")
			}
			var schema map[string]any
			if err := json.Unmarshal(tc.tool.Schema, &schema); err != nil {
				t.Errorf("schema is not valid JSON: %v", err)
			}
		})
	}
}

// ---------- helper funcs ----------

func TestArrayLen(t *testing.T) {
	cases := []struct {
		name string
		data any
		key  string
		want int
	}{
		{"array present", map[string]any{"xs": []any{1, 2, 3}}, "xs", 3},
		{"empty array", map[string]any{"xs": []any{}}, "xs", 0},
		{"key missing", map[string]any{"ys": []any{1}}, "xs", 0},
		{"value not array", map[string]any{"xs": "nope"}, "xs", 0},
		{"data not map", "scalar", "xs", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := arrayLen(tc.data, tc.key); got != tc.want {
				t.Errorf("arrayLen=%d, want %d", got, tc.want)
			}
		})
	}
}

func TestStringField(t *testing.T) {
	cases := []struct {
		name string
		data any
		key  string
		want string
	}{
		{"string present", map[string]any{"s": "hi"}, "s", "hi"},
		{"key missing", map[string]any{"t": "hi"}, "s", ""},
		{"value not string", map[string]any{"s": 42}, "s", ""},
		{"data not map", []any{1}, "s", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stringField(tc.data, tc.key); got != tc.want {
				t.Errorf("stringField=%q, want %q", got, tc.want)
			}
		})
	}
}
