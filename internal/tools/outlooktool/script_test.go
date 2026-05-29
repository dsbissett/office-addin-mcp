package outlooktool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRunRunScript_HappyPath(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"value": 42}))
	res := runRunScript(context.Background(),
		json.RawMessage(`{"script":"return 42;"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if res.Summary != "Ran custom Outlook script." {
		t.Errorf("summary=%q", res.Summary)
	}
}

func TestRunRunScript_WithScriptArgs(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"value": 1}))
	res := runRunScript(context.Background(),
		json.RawMessage(`{"script":"return args.x;","scriptArgs":{"x":1}}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}

func TestRunRunScript_OfficeError(t *testing.T) {
	env := fakeEnv(t, officeErrReply("UnexpectedError", "boom", nil))
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"throw 1;"}`), env)
	if res.Err == nil || res.Err.Category != tools.CategoryOfficeJS || res.Err.Code != "UnexpectedError" {
		t.Fatalf("want office_js/UnexpectedError, got %+v", res.Err)
	}
}

// An Office.js error with an empty code falls back to the "office_js_error"
// default code (codeOrDefault branch).
func TestRunRunScript_OfficeError_EmptyCodeDefaults(t *testing.T) {
	env := fakeEnv(t, officeErrReply("", "no code", nil))
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"x"}`), env)
	if res.Err == nil || res.Err.Code != "office_js_error" {
		t.Fatalf("want default code office_js_error, got %+v", res.Err)
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

// A JS exceptionDetails from Runtime.evaluate surfaces as a protocol exception.
func TestRunRunScript_ProtocolException(t *testing.T) {
	env := fakeEnv(t, protocolExceptionReply("Uncaught ReferenceError: foo is not defined"))
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"foo()"}`), env)
	if res.Err == nil || res.Err.Code != "payload_protocol_exception" {
		t.Fatalf("want payload_protocol_exception, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryProtocol {
		t.Errorf("category=%q want protocol", res.Err.Category)
	}
}

// A CDP-level RemoteError from Runtime.evaluate surfaces as payload_failed.
func TestRunRunScript_CDPError(t *testing.T) {
	env := fakeEnv(t, cdpErrReply(-32000, "session gone"))
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"x"}`), env)
	if res.Err == nil || res.Err.Code != "payload_failed" {
		t.Fatalf("want payload_failed, got %+v", res.Err)
	}
	// CDP RemoteError details are surfaced for branching.
	if res.Err.Details["cdpError"] == nil {
		t.Errorf("expected cdpError in details, got %+v", res.Err.Details)
	}
}

// To exercise the decode_payload_result failure branch we hand back a value
// that is neither an Office error nor carries a "result" field. The executor
// decodes the envelope (Result=nil, OfficeError=false) and returns a nil
// payload; RunPayload's json.Unmarshal(nil) then fails with decode_payload_result.
func TestRunRunScript_DecodePayloadResultFailure(t *testing.T) {
	resp := func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		if method == "Runtime.evaluate" {
			return cdptest.Eval(map[string]any{"unexpected": "shape"}), nil
		}
		return map[string]any{}, nil
	}
	env := fakeEnv(t, resp)
	res := runRunScript(context.Background(), json.RawMessage(`{"script":"x"}`), env)
	if res.Err == nil || res.Err.Code != "decode_payload_result" {
		t.Fatalf("want decode_payload_result, got %+v", res.Err)
	}
	if res.Err.Category != tools.CategoryInternal {
		t.Errorf("category=%q want internal", res.Err.Category)
	}
}

func TestRunScriptTool_RunWiring(t *testing.T) {
	env := fakeEnv(t, officeReply(map[string]any{"ok": true}))
	res := RunScript().Run(context.Background(), json.RawMessage(`{"script":"return 1;"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}
