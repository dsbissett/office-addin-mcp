package outlooktool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// ---------- outlook.readItem ----------

func TestRunReadItem_WithSubject(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"subject": "Quarterly report", "itemType": "message"}))
	res := runReadItem(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read item: Quarterly report" {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadItem_EmptySubject(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"itemType": "message"}))
	res := runReadItem(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read mailbox item." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunReadItem_BadParams(t *testing.T) {
	res := runReadItem(context.Background(), json.RawMessage(`{"targetId":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunReadItem_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrReply("ItemNotFound", "no item selected", nil))
	res := runReadItem(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js, got %+v", res.Err)
	}
}

func TestReadItemTool_RunWiring(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"subject": "x"}))
	res := ReadItem().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}

// ---------- outlook.getBody ----------

func TestRunGetBody_Default(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"body": "hello world", "coercionType": "text"}))
	res := runGetBody(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read item body (11 chars, text)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunGetBody_HTMLCoercion(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"body": "<b>x</b>", "coercionType": "html"}))
	res := runGetBody(context.Background(), json.RawMessage(`{"coercionType":"html"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read item body (8 chars, html)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunGetBody_BadParams(t *testing.T) {
	res := runGetBody(context.Background(), json.RawMessage(`{"coercionType":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestGetBodyTool_RunWiring(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"body": "", "coercionType": "text"}))
	res := GetBody().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}

// ---------- outlook.setBody ----------

func TestRunSetBody_Default(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"ok": true}))
	res := runSetBody(context.Background(), json.RawMessage(`{"content":"hello"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Set item body (5 chars)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunSetBody_WithCoercion(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"ok": true}))
	res := runSetBody(context.Background(),
		json.RawMessage(`{"content":"<p>hi</p>","coercionType":"html"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Set item body (9 chars)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunSetBody_BadParams(t *testing.T) {
	res := runSetBody(context.Background(), json.RawMessage(`{"content":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunSetBody_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrReply("InvalidOperation", "read mode", nil))
	res := runSetBody(context.Background(), json.RawMessage(`{"content":"x"}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS {
		t.Fatalf("want office_js, got %+v", res.Err)
	}
}

func TestSetBodyTool_RunWiring(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"ok": true}))
	res := SetBody().Run(context.Background(), json.RawMessage(`{"content":"x"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}

// ---------- outlook.getSubject ----------

func TestRunGetSubject_WithSubject(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"subject": "Hello", "mode": "read"}))
	res := runGetSubject(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read subject (mode=read): Hello" {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunGetSubject_EmptySubject(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"subject": "", "mode": "compose"}))
	res := runGetSubject(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read subject (empty, mode=compose)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunGetSubject_BadParams(t *testing.T) {
	res := runGetSubject(context.Background(), json.RawMessage(`{"urlPattern":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestGetSubjectTool_RunWiring(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"subject": "x", "mode": "read"}))
	res := GetSubject().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}

// ---------- outlook.setSubject ----------

func TestRunSetSubject_Happy(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"ok": true}))
	res := runSetSubject(context.Background(), json.RawMessage(`{"subject":"New subject"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Set subject to: New subject" {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunSetSubject_BadParams(t *testing.T) {
	res := runSetSubject(context.Background(), json.RawMessage(`{"subject":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestRunSetSubject_AttachFailure(t *testing.T) {
	res := runSetSubject(context.Background(), json.RawMessage(`{"subject":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestSetSubjectTool_RunWiring(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"ok": true}))
	res := SetSubject().Run(context.Background(), json.RawMessage(`{"subject":"x"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}

// ---------- outlook.getRecipients ----------

func TestRunGetRecipients_Counts(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{
		"to": []any{map[string]any{"emailAddress": "a@x.com"}, map[string]any{"emailAddress": "b@x.com"}},
		"cc": []any{map[string]any{"emailAddress": "c@x.com"}},
	}))
	res := runGetRecipients(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read 2 To and 1 Cc recipient(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunGetRecipients_Empty(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"to": []any{}, "cc": []any{}}))
	res := runGetRecipients(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Read 0 To and 0 Cc recipient(s)." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunGetRecipients_BadParams(t *testing.T) {
	res := runGetRecipients(context.Background(), json.RawMessage(`{"targetId":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestGetRecipientsTool_RunWiring(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"to": []any{}, "cc": []any{}}))
	res := GetRecipients().Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}
