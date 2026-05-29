package inspecttool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRunWaitFor_SatisfiedImmediately(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
			return evalResult("boolean", "true"), nil
		},
	})
	res := runWaitFor(context.Background(), json.RawMessage(`{"expression":"document.ready"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out, ok := res.Data.(struct {
		Satisfied bool `json:"satisfied"`
		Attempts  int  `json:"attempts"`
	})
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if !out.Satisfied || out.Attempts != 1 {
		t.Errorf("out=%+v, want satisfied after 1 attempt", out)
	}
}

func TestRunWaitFor_Timeout(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			// Predicate never becomes truthy.
			return evalResult("boolean", "false"), nil
		},
	})
	// Tiny timeout + interval so the loop exits quickly.
	res := runWaitFor(context.Background(), json.RawMessage(`{"expression":"x","timeoutMs":1,"intervalMs":1}`), env)
	if res.Err == nil {
		t.Fatal("expected timeout error")
	}
	if res.Err.Code != "wait_timeout" || res.Err.Category != tools.CategoryTimeout {
		t.Errorf("err=%+v, want wait_timeout/timeout", res.Err)
	}
	if !res.Err.Retryable {
		t.Errorf("wait_timeout should be retryable")
	}
}

func TestRunWaitFor_EvaluateError(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return nil, &cdp.RemoteError{Code: -32000, Message: "session gone"}
		},
	})
	res := runWaitFor(context.Background(), json.RawMessage(`{"expression":"x"}`), env)
	if res.Err == nil || res.Err.Code != "evaluate_failed" {
		t.Fatalf("want evaluate_failed, got %+v", res.Err)
	}
}

func TestRunWaitFor_ContextCanceled(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return evalResult("boolean", "false"), nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call so the select hits ctx.Done after the first poll
	// Large interval ensures we wait in the select, where ctx.Done fires first.
	res := runWaitFor(ctx, json.RawMessage(`{"expression":"x","timeoutMs":100000,"intervalMs":100000}`), env)
	if res.Err == nil {
		t.Fatal("expected an error from a canceled context")
	}
}

func TestRunWaitFor_AttachFailure(t *testing.T) {
	res := runWaitFor(context.Background(), json.RawMessage(`{"expression":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunWaitFor_BadParams(t *testing.T) {
	res := runWaitFor(context.Background(), json.RawMessage(`{"expression":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestWaitFor_ToolMetadata(t *testing.T) {
	tool := WaitFor()
	if tool.Name != "page.waitFor" || tool.Run == nil {
		t.Errorf("unexpected tool metadata: %+v", tool)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
}
