package inspecttool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// evalResult builds a Runtime.evaluate CDP reply with the given RemoteObject
// type and raw JSON value.
func evalResult(typ, value string) map[string]any {
	r := map[string]any{"type": typ}
	if value != "" {
		r["value"] = json.RawMessage(value)
	}
	return map[string]any{"result": r}
}

func TestRunEvaluate_HappyPath(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
			if method != "Runtime.evaluate" {
				t.Errorf("unexpected method %q", method)
			}
			return evalResult("string", `"hello"`), nil
		},
	})
	res := runEvaluate(context.Background(), json.RawMessage(`{"expression":"1+1"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Evaluated JS; result type string." {
		t.Errorf("summary=%q", res.Summary)
	}
	out, ok := res.Data.(struct {
		Type        string          `json:"type"`
		Value       json.RawMessage `json:"value,omitempty"`
		Description string          `json:"description,omitempty"`
	})
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if out.Type != "string" || string(out.Value) != `"hello"` {
		t.Errorf("out=%+v", out)
	}
}

func TestRunEvaluate_ReturnByValueFalse(t *testing.T) {
	var sawReturnByValue bool
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
			var p struct {
				ReturnByValue bool `json:"returnByValue"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				t.Fatalf("decode params: %v", err)
			}
			sawReturnByValue = p.ReturnByValue
			return evalResult("number", "42"), nil
		},
	})
	res := runEvaluate(context.Background(), json.RawMessage(`{"expression":"x","returnByValue":false}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if sawReturnByValue {
		t.Errorf("returnByValue should have been passed false to CDP")
	}
}

func TestRunEvaluate_AwaitPromiseUndefinedWarns(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return evalResult("undefined", ""), nil
		},
	})
	res := runEvaluate(context.Background(), json.RawMessage(`{"expression":"doWork()","awaitPromise":true}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary == "" || res.Summary == "Evaluated JS; result type undefined." {
		t.Errorf("expected the undefined-promise warning summary, got %q", res.Summary)
	}
}

func TestRunEvaluate_FetchFailureException(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return map[string]any{
				"result": map[string]any{"type": "undefined"},
				"exceptionDetails": map[string]any{
					"text":      "Uncaught (in promise)",
					"exception": map[string]any{"type": "object", "description": "TypeError: Failed to fetch"},
				},
			}, nil
		},
	})
	res := runEvaluate(context.Background(), json.RawMessage(`{"expression":"fetch('/x')","awaitPromise":true}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Code != "fetch_failed" || res.Err.Category != tools.CategoryConnection {
		t.Errorf("err=%+v, want fetch_failed/connection", res.Err)
	}
	if !res.Err.Retryable {
		t.Errorf("fetch_failed should be retryable")
	}
	if res.Err.Details["recoverableViaTool"] != "addin.ensureRunning" {
		t.Errorf("missing recoverableViaTool detail: %+v", res.Err.Details)
	}
}

func TestRunEvaluate_GenericException(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return map[string]any{
				"result": map[string]any{"type": "undefined"},
				"exceptionDetails": map[string]any{
					"text":      "Uncaught",
					"exception": map[string]any{"type": "object", "description": "ReferenceError: foo is not defined"},
				},
			}, nil
		},
	})
	res := runEvaluate(context.Background(), json.RawMessage(`{"expression":"foo"}`), env)
	if res.Err == nil {
		t.Fatal("expected error")
	}
	if res.Err.Code != "evaluation_exception" || res.Err.Category != tools.CategoryProtocol {
		t.Errorf("err=%+v, want evaluation_exception/protocol", res.Err)
	}
}

func TestRunEvaluate_CDPSendError(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return nil, &cdp.RemoteError{Code: -32000, Message: "boom"}
		},
	})
	res := runEvaluate(context.Background(), json.RawMessage(`{"expression":"x"}`), env)
	if res.Err == nil || res.Err.Code != "evaluate_failed" {
		t.Fatalf("want evaluate_failed, got %+v", res.Err)
	}
}

func TestRunEvaluate_AttachFailure(t *testing.T) {
	res := runEvaluate(context.Background(), json.RawMessage(`{"expression":"x"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
	if res.Err.Details["recoverableViaTool"] != "addin.ensureRunning" {
		t.Errorf("missing recoverableViaTool detail: %+v", res.Err.Details)
	}
}

func TestRunEvaluate_BadParams(t *testing.T) {
	res := runEvaluate(context.Background(), json.RawMessage(`{"expression":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestEvaluate_ToolMetadata(t *testing.T) {
	tool := Evaluate()
	if tool.Name != "page.evaluate" {
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

func TestIsFetchFailure(t *testing.T) {
	hits := []string{
		"TypeError: Failed to fetch",
		"Uncaught (in promise) TypeError: Failed to fetch\n    at uploadPdfs",
		"NetworkError when attempting to fetch resource.",
		"net::ERR_CONNECTION_REFUSED",
		"Load failed",
	}
	for _, m := range hits {
		if !isFetchFailure(m) {
			t.Errorf("isFetchFailure(%q) = false, want true", m)
		}
	}
	misses := []string{
		"TypeError: undefined is not a function",
		"ReferenceError: foo is not defined",
		"SyntaxError: Unexpected token",
		"",
	}
	for _, m := range misses {
		if isFetchFailure(m) {
			t.Errorf("isFetchFailure(%q) = true, want false", m)
		}
	}
}
