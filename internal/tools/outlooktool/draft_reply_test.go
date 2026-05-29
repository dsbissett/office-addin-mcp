package outlooktool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRunDraftReply_SubjectAndBody(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"ok": true}))
	res := runDraftReply(context.Background(),
		json.RawMessage(`{"subject":"Re: hi","body":"<p>hello</p>"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Drafted reply: set subject + body." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunDraftReply_SubjectOnly(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"ok": true}))
	res := runDraftReply(context.Background(), json.RawMessage(`{"subject":"Re: hi"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Drafted reply: set subject." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunDraftReply_BodyOnly_TextCoercion(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"ok": true}))
	res := runDraftReply(context.Background(),
		json.RawMessage(`{"body":"plain","coercionType":"TEXT"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Drafted reply: set body." {
		t.Errorf("summary=%q", res.Summary)
	}
}

// An unrecognized coercionType is silently dropped from the args (neither
// "html" nor "text") — the call still succeeds.
func TestRunDraftReply_UnknownCoercionDropped(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"ok": true}))
	res := runDraftReply(context.Background(),
		json.RawMessage(`{"subject":"x","coercionType":"rtf"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}

func TestRunDraftReply_NothingToSet(t *testing.T) {
	res := runDraftReply(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "nothing_to_set" {
		t.Fatalf("want nothing_to_set, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryValidation {
		t.Errorf("category=%q", res.Err.Category)
	}
}

func TestRunDraftReply_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrReply("InvalidOperation", "compose only", map[string]any{"errorLocation": "Body.setAsync"}))
	res := runDraftReply(context.Background(), json.RawMessage(`{"body":"x"}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "InvalidOperation" {
		t.Errorf("err=%+v, want office_js/InvalidOperation", res.Err)
	}
	if res.Summary != "Office.js error: compose only" {
		t.Errorf("summary=%q", res.Summary)
	}
	// debugInfo flows into Details.
	if res.Err.Details["debugInfo"] == nil {
		t.Errorf("expected debugInfo in details, got %+v", res.Err.Details)
	}
}

func TestRunDraftReply_AttachFailure(t *testing.T) {
	res := runDraftReply(context.Background(), json.RawMessage(`{"subject":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryNotFound {
		t.Errorf("category=%q want not_found", res.Err.Category)
	}
}

func TestRunDraftReply_BadParams(t *testing.T) {
	res := runDraftReply(context.Background(), json.RawMessage(`{"subject":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// DraftReply().Run is the registered entry point; exercising it confirms the
// constructor wires runDraftReply.
func TestDraftReplyTool_RunWiring(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"ok": true}))
	res := DraftReply().Run(context.Background(), json.RawMessage(`{"subject":"hi"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}
