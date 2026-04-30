package officejs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

type mockEvaluator struct {
	fn func(ctx context.Context, sessionID string, p cdp.EvaluateParams) (*cdp.EvaluateResult, error)
}

func (m *mockEvaluator) Evaluate(ctx context.Context, sessionID string, p cdp.EvaluateParams) (*cdp.EvaluateResult, error) {
	return m.fn(ctx, sessionID, p)
}

// returnValue builds an EvaluateResult that mimics Runtime.evaluate returning
// `returnByValue:true` — Result.Value is the JSON of the JS return value.
func returnValue(jsonText string) *cdp.EvaluateResult {
	return &cdp.EvaluateResult{
		Result: &cdp.RemoteObject{
			Type:  "object",
			Value: json.RawMessage(jsonText),
		},
	}
}

func TestExecutor_SuccessUnwrap(t *testing.T) {
	var capturedExpr string
	var capturedSession string
	mock := &mockEvaluator{
		fn: func(_ context.Context, sessionID string, p cdp.EvaluateParams) (*cdp.EvaluateResult, error) {
			capturedSession = sessionID
			capturedExpr = p.Expression
			return returnValue(`{"result":{"answer":42}}`), nil
		},
	}
	exec := New(mock, "sess-1")

	raw, err := exec.Run(context.Background(), "excel.readRange", map[string]any{"address": "A1"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var got struct{ Answer int }
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Answer != 42 {
		t.Errorf("answer=%d", got.Answer)
	}
	if capturedSession != "sess-1" {
		t.Errorf("session=%q", capturedSession)
	}
	for _, want := range []string{"async", "args", `"address":"A1"`, "__runExcel"} {
		if !strings.Contains(capturedExpr, want) {
			t.Errorf("expression missing %q", want)
		}
	}
}

func TestExecutor_OfficeErrorUnwrap(t *testing.T) {
	mock := &mockEvaluator{
		fn: func(_ context.Context, _ string, _ cdp.EvaluateParams) (*cdp.EvaluateResult, error) {
			return returnValue(`{"__officeError":true,"code":"ItemNotFound","message":"Worksheet 'X' not found","debugInfo":{"errorLocation":"worksheets.getItem"}}`), nil
		},
	}
	exec := New(mock, "sess-1")

	_, err := exec.Run(context.Background(), "excel.readRange", map[string]any{"address": "A1"})
	if err == nil {
		t.Fatal("expected error")
	}
	var oerr *OfficeError
	if !errors.As(err, &oerr) {
		t.Fatalf("expected *OfficeError, got %T: %v", err, err)
	}
	if oerr.Code != "ItemNotFound" {
		t.Errorf("code=%q", oerr.Code)
	}
	if !strings.Contains(string(oerr.DebugInfo), "errorLocation") {
		t.Errorf("debugInfo missing: %s", oerr.DebugInfo)
	}
}

func TestExecutor_ProtocolException(t *testing.T) {
	mock := &mockEvaluator{
		fn: func(_ context.Context, _ string, _ cdp.EvaluateParams) (*cdp.EvaluateResult, error) {
			return &cdp.EvaluateResult{
				ExceptionDetails: &cdp.ExceptionDetails{
					Text: "Uncaught SyntaxError: Unexpected token",
				},
			}, nil
		},
	}
	exec := New(mock, "sess-1")

	_, err := exec.Run(context.Background(), "excel.readRange", map[string]any{"address": "A1"})
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *ProtocolException
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ProtocolException, got %T: %v", err, err)
	}
}

func TestExecutor_UnknownPayload(t *testing.T) {
	exec := New(&mockEvaluator{fn: nil}, "sess-1")
	_, err := exec.Run(context.Background(), "excel.bogus", nil)
	if err == nil || !strings.Contains(err.Error(), "no payload") {
		t.Fatalf("expected no-payload error, got %v", err)
	}
}

func TestExecutor_TransportErrorPropagated(t *testing.T) {
	mock := &mockEvaluator{
		fn: func(_ context.Context, _ string, _ cdp.EvaluateParams) (*cdp.EvaluateResult, error) {
			return nil, errors.New("ws closed")
		},
	}
	exec := New(mock, "sess-1")
	_, err := exec.Run(context.Background(), "excel.readRange", map[string]any{"address": "A1"})
	if err == nil || !strings.Contains(err.Error(), "ws closed") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestEncodeArgs_EscapesU2028(t *testing.T) {
	// json.Marshal escapes U+2028 / U+2029 (line/paragraph separator) to
	// JS-safe \uXXXX sequences so the resulting JSON literal parses inside a
	// JS expression. We only need to assert the raw codepoints are gone — the
	// exact escape form is encoding/json's choice.
	v := map[string]string{
		"a": string(rune(0x2028)),
		"b": string(rune(0x2029)),
	}
	got, err := encodeArgs(v)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if strings.ContainsRune(got, 0x2028) {
		t.Errorf("U+2028 not escaped: %q", got)
	}
	if strings.ContainsRune(got, 0x2029) {
		t.Errorf("U+2029 not escaped: %q", got)
	}
}
