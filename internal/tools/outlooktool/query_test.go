package outlooktool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRunQuery_HappyPath_NoQuery(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"rows": []any{
		map[string]any{"subject": "a"},
		map[string]any{"subject": "b"},
	}}))
	res := runQuery(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Query returned 2 row(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunQuery_WithQuery_NoRows(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"rows": []any{}}))
	res := runQuery(context.Background(),
		json.RawMessage(`{"query":{"filter":{"field":"subject","op":"contains","value":"x"},"limit":5}}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Query returned 0 row(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunQuery_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrReply("InvalidArgument", "bad filter", nil))
	res := runQuery(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "InvalidArgument" {
		t.Fatalf("want office_js/InvalidArgument, got %+v", res.Err)
	}
}

func TestRunQuery_AttachFailure(t *testing.T) {
	res := runQuery(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunQuery_BadParams(t *testing.T) {
	// targetId is a string field; a number there fails the struct decode.
	res := runQuery(context.Background(), json.RawMessage(`{"targetId":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestQueryTool_RunWiring(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"rows": []any{}}))
	res := Query().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}
