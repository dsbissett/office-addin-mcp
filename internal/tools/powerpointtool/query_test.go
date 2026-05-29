package powerpointtool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRunQuery_HappyPath(t *testing.T) {
	env := fakeEnv(t, officeOK(map[string]any{
		"rows": []any{
			map[string]any{"name": "Title 1"},
			map[string]any{"name": "Content"},
		},
	}))
	res := runQuery(context.Background(),
		json.RawMessage(`{"query":{"filter":{"col":"type","eq":"GeometricShape"}}}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Query returned 2 row(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunQuery_NoQueryBody(t *testing.T) {
	// Empty params → no "query" arg appended; summary still computes from rows.
	env := fakeEnv(t, officeOK(map[string]any{"rows": []any{}}))
	res := runQuery(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Query returned 0 row(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunQuery_NonMapData(t *testing.T) {
	// Payload returns a non-object → arrayLen returns 0.
	env := fakeEnv(t, officeOK([]any{1, 2, 3}))
	res := runQuery(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Query returned 0 row(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunQuery_OfficeError(t *testing.T) {
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

func TestRunQuery_AttachFailure(t *testing.T) {
	res := runQuery(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunQuery_BadParams(t *testing.T) {
	// query is json.RawMessage (any JSON decodes), so force the decode error via
	// a wrong-typed selector field instead.
	res := runQuery(context.Background(), json.RawMessage(`{"targetId":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}
