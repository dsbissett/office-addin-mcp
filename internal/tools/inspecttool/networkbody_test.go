package inspecttool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

func TestRunNetworkBody_HappyUTF8(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
			if method == "Network.getResponseBody" {
				var p struct {
					RequestID string `json:"requestId"`
				}
				if err := json.Unmarshal(params, &p); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if p.RequestID != "req-1" {
					t.Errorf("requestId=%q", p.RequestID)
				}
			}
			return map[string]any{"body": "hello body", "base64Encoded": false}, nil
		},
	})
	res := runNetworkBody(context.Background(), json.RawMessage(`{"requestId":"req-1"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out, ok := res.Data.(struct {
		RequestID     string `json:"requestId"`
		Body          string `json:"body"`
		Base64Encoded bool   `json:"base64Encoded"`
	})
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if out.Body != "hello body" || out.Base64Encoded {
		t.Errorf("out=%+v", out)
	}
	if !strings.Contains(res.Summary, "utf-8") {
		t.Errorf("summary=%q, want utf-8 mention", res.Summary)
	}
}

func TestRunNetworkBody_HappyBase64(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return map[string]any{"body": "aGVsbG8=", "base64Encoded": true}, nil
		},
	})
	res := runNetworkBody(context.Background(), json.RawMessage(`{"requestId":"req-1"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !strings.Contains(res.Summary, "base64") {
		t.Errorf("summary=%q, want base64 mention", res.Summary)
	}
}

func TestRunNetworkBody_TooLarge(t *testing.T) {
	big := strings.Repeat("a", networkBodyMaxBytes+1)
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return map[string]any{"body": big, "base64Encoded": false}, nil
		},
	})
	res := runNetworkBody(context.Background(), json.RawMessage(`{"requestId":"req-1"}`), env)
	if res.Err == nil || res.Err.Code != "body_too_large" {
		t.Fatalf("want body_too_large, got %+v", res.Err)
	}
	if res.Err.Details["cap"] != networkBodyMaxBytes {
		t.Errorf("missing cap detail: %+v", res.Err.Details)
	}
}

func TestRunNetworkBody_BodyDecodeError(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			// body is a number → body_decode.
			return map[string]any{"body": 123, "base64Encoded": false}, nil
		},
	})
	res := runNetworkBody(context.Background(), json.RawMessage(`{"requestId":"req-1"}`), env)
	if res.Err == nil || res.Err.Code != "body_decode" {
		t.Fatalf("want body_decode, got %+v", res.Err)
	}
}

func TestRunNetworkBody_EnableNetworkFailed(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		enableErr: errors.New("enable network broke"),
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return map[string]any{"body": "x"}, nil
		},
	})
	res := runNetworkBody(context.Background(), json.RawMessage(`{"requestId":"req-1"}`), env)
	if res.Err == nil || res.Err.Code != "enable_network_failed" {
		t.Fatalf("want enable_network_failed, got %+v", res.Err)
	}
}

func TestRunNetworkBody_SendError(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return nil, &cdp.RemoteError{Code: -32000, Message: "no such request"}
		},
	})
	res := runNetworkBody(context.Background(), json.RawMessage(`{"requestId":"missing"}`), env)
	if res.Err == nil || res.Err.Code != "get_response_body_failed" {
		t.Fatalf("want get_response_body_failed, got %+v", res.Err)
	}
}

func TestRunNetworkBody_AttachFailure(t *testing.T) {
	res := runNetworkBody(context.Background(), json.RawMessage(`{"requestId":"r"}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunNetworkBody_BadParams(t *testing.T) {
	res := runNetworkBody(context.Background(), json.RawMessage(`{"requestId":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestNetworkBody_ToolMetadata(t *testing.T) {
	tool := NetworkBody()
	if tool.Name != "page.networkBody" || tool.Run == nil {
		t.Errorf("unexpected tool metadata: %+v", tool)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
}
