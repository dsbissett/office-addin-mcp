package onenotetool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestQuery_HappyPath_RowCount(t *testing.T) {
	env := fakeEnv(t, okOffice(map[string]any{
		"rows": []any{
			map[string]any{"id": "1", "title": "A"},
			map[string]any{"id": "2", "title": "B"},
		},
	}))
	res := runQuery(context.Background(),
		json.RawMessage(`{"query":{"limit":5}}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Query returned 2 row(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestQuery_NoQueryArg_ZeroRows(t *testing.T) {
	// Omitting the query field exercises the len(p.Query)==0 branch and the
	// arrayLen fallback when "rows" is absent.
	env := fakeEnv(t, okOffice(map[string]any{}))
	res := runQuery(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Query returned 0 row(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestQuery_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErr("InvalidArgument", "bad filter"))
	res := runQuery(context.Background(),
		json.RawMessage(`{"query":{"filter":{}}}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "InvalidArgument" {
		t.Errorf("err=%+v, want office_js/InvalidArgument", res.Err)
	}
}

func TestQuery_AttachFailure(t *testing.T) {
	res := runQuery(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestQuery_BadParams(t *testing.T) {
	// query is json.RawMessage (accepts any token), so force the decode failure
	// via a non-string selector field instead.
	res := runQuery(context.Background(),
		json.RawMessage(`{"targetId":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestQuery_ToolDefinition(t *testing.T) {
	tool := Query()
	if tool.Name != "onenote.query" {
		t.Errorf("name=%q", tool.Name)
	}
	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Error("query should be read-only")
	}
	if tool.Run == nil {
		t.Error("Run is nil")
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
}
