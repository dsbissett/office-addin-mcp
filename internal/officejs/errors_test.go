package officejs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

func TestOfficeError_Error(t *testing.T) {
	withMsg := &OfficeError{Code: "ItemNotFound", Message: "Worksheet 'X' not found"}
	got := withMsg.Error()
	if !strings.Contains(got, "Worksheet 'X' not found") || !strings.Contains(got, "ItemNotFound") {
		t.Errorf("Error() = %q, want message+code", got)
	}
	if !strings.HasPrefix(got, "office.js:") {
		t.Errorf("Error() = %q, want office.js prefix", got)
	}

	noMsg := &OfficeError{Code: "GeneralException"}
	got = noMsg.Error()
	if !strings.Contains(got, "GeneralException") {
		t.Errorf("Error() = %q, want code", got)
	}
	if strings.Contains(got, "(") {
		t.Errorf("Error() = %q, expected no parenthesized message for empty Message", got)
	}
}

func TestProtocolException_Error(t *testing.T) {
	pe := &ProtocolException{Text: "Uncaught SyntaxError"}
	got := pe.Error()
	if !strings.Contains(got, "Uncaught SyntaxError") {
		t.Errorf("Error() = %q, want text", got)
	}
	if !strings.Contains(got, "protocol exception") {
		t.Errorf("Error() = %q, want protocol exception label", got)
	}
}

// TestExecutor_EmptyValue covers the branch where Runtime.evaluate returns a
// Result with no Value (empty payload return), reporting the type in the error.
func TestExecutor_EmptyValue(t *testing.T) {
	mock := &mockEvaluator{
		fn: func(_ context.Context, _ string, _ cdp.EvaluateParams) (*cdp.EvaluateResult, error) {
			return &cdp.EvaluateResult{
				Result: &cdp.RemoteObject{Type: "undefined"},
			}, nil
		},
	}
	exec := New(mock, "sess-1")
	_, err := exec.Run(context.Background(), "excel.readRange", map[string]any{"address": "A1"})
	if err == nil || !strings.Contains(err.Error(), "empty value") {
		t.Fatalf("expected empty value error, got %v", err)
	}
	if !strings.Contains(err.Error(), "undefined") {
		t.Errorf("expected type undefined in error, got %v", err)
	}
}

// TestExecutor_NilResult covers the branch where Result itself is nil.
func TestExecutor_NilResult(t *testing.T) {
	mock := &mockEvaluator{
		fn: func(_ context.Context, _ string, _ cdp.EvaluateParams) (*cdp.EvaluateResult, error) {
			return &cdp.EvaluateResult{Result: nil}, nil
		},
	}
	exec := New(mock, "sess-1")
	_, err := exec.Run(context.Background(), "excel.readRange", nil)
	if err == nil || !strings.Contains(err.Error(), "empty value") {
		t.Fatalf("expected empty value error, got %v", err)
	}
	// With a nil Result the type is the empty string.
	if !strings.Contains(err.Error(), `type=""`) {
		t.Errorf("expected empty type in error, got %v", err)
	}
}

// TestExecutor_DecodeEnvelopeError covers the branch where the payload return
// value is not valid JSON for the envelope struct.
func TestExecutor_DecodeEnvelopeError(t *testing.T) {
	mock := &mockEvaluator{
		fn: func(_ context.Context, _ string, _ cdp.EvaluateParams) (*cdp.EvaluateResult, error) {
			return returnValue(`not-json`), nil
		},
	}
	exec := New(mock, "sess-1")
	_, err := exec.Run(context.Background(), "excel.readRange", nil)
	if err == nil || !strings.Contains(err.Error(), "decode payload envelope") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

// TestExecutor_MarshalArgsError covers the encodeArgs error path inside Run by
// passing a value that encoding/json cannot marshal (a channel).
func TestExecutor_MarshalArgsError(t *testing.T) {
	mock := &mockEvaluator{
		fn: func(_ context.Context, _ string, _ cdp.EvaluateParams) (*cdp.EvaluateResult, error) {
			t.Fatal("Evaluate should not be reached when args fail to marshal")
			return nil, nil
		},
	}
	exec := New(mock, "sess-1")
	_, err := exec.Run(context.Background(), "excel.readRange", make(chan int))
	if err == nil || !strings.Contains(err.Error(), "marshal args") {
		t.Fatalf("expected marshal args error, got %v", err)
	}
}

// TestEncodeArgs_MarshalError directly exercises encodeArgs's error branch.
func TestEncodeArgs_MarshalError(t *testing.T) {
	_, err := encodeArgs(make(chan int))
	if err == nil || !strings.Contains(err.Error(), "marshal args") {
		t.Fatalf("expected marshal args error, got %v", err)
	}
}

// TestEncodeArgs_Roundtrip covers the happy path of encodeArgs producing valid
// JSON that round-trips back to the original map.
func TestEncodeArgs_Roundtrip(t *testing.T) {
	in := map[string]any{"address": "A1:B2", "n": 3}
	got, err := encodeArgs(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal([]byte(got), &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back["address"] != "A1:B2" {
		t.Errorf("address=%v", back["address"])
	}
}

// TestExecutor_RunUnknownTransportNotInvoked ensures the getPayload error path
// short-circuits before Evaluate is called.
func TestExecutor_RunUnknownTransportNotInvoked(t *testing.T) {
	called := false
	mock := &mockEvaluator{
		fn: func(_ context.Context, _ string, _ cdp.EvaluateParams) (*cdp.EvaluateResult, error) {
			called = true
			return nil, errors.New("should not run")
		},
	}
	exec := New(mock, "sess-1")
	if _, err := exec.Run(context.Background(), "excel.doesNotExist", nil); err == nil {
		t.Fatal("expected error for unknown payload")
	}
	if called {
		t.Error("Evaluate should not be called for unknown payload")
	}
}
