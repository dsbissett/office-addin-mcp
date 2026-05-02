package tools

import (
	"context"
	"encoding/json"
	"testing"
)

// BenchmarkDispatchNoSession measures the dispatcher's per-call overhead on
// the NoSession fast path: tool lookup → schema validation → request-id
// generation → finalize. No CDP connection, no Acquire, no goroutines —
// this is the floor any other call has to clear, so it's the right baseline
// to track for regressions in the dispatcher itself.
func BenchmarkDispatchNoSession(b *testing.B) {
	reg := NewRegistry()
	reg.MustRegister(fakeTool())
	req := Request{
		Tool:   "fake.run",
		Params: []byte(`{"mode":"ok"}`),
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env := Dispatch(ctx, reg, req)
		if !env.OK {
			b.Fatalf("dispatch failed: %+v", env.Error)
		}
	}
}

// BenchmarkDispatchValidationError measures the schema-violation path
// because it's hit on every malformed tool call and we want it cheap. The
// dispatcher returns before any session work — useful baseline for "how
// expensive is santhosh-tekuri/jsonschema validation".
func BenchmarkDispatchValidationError(b *testing.B) {
	reg := NewRegistry()
	reg.MustRegister(fakeTool())
	req := Request{
		Tool:   "fake.run",
		Params: []byte(`{}`), // missing required "mode"
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		env := Dispatch(ctx, reg, req)
		if env.OK {
			b.Fatal("expected validation error, got OK")
		}
	}
}

// BenchmarkMarshalEnvelope measures the JSON encoding step the adapter
// hits on every result. Includes the diagnostics block, so this tracks the
// wire-format cost the agent ultimately pays.
func BenchmarkMarshalEnvelope(b *testing.B) {
	env := Envelope{
		OK:   true,
		Data: map[string]any{"answer": 42, "nested": map[string]any{"x": 1, "y": 2}},
		Diagnostics: Diagnostics{
			Tool:            "fake.run",
			EnvelopeVersion: EnvelopeVersion,
			RequestID:       "deadbeefcafef00d",
			SessionID:       "default",
			DurationMs:      3,
			CDPRoundTrips:   1,
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := MarshalEnvelope(env)
		if err != nil {
			b.Fatalf("marshal: %v", err)
		}
		if !json.Valid(out) {
			b.Fatal("invalid json")
		}
	}
}
