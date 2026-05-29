package log

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestWithRequestIDRoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-123")
	if got := RequestID(ctx); got != "req-123" {
		t.Errorf("RequestID = %q, want %q", got, "req-123")
	}
}

func TestWithRequestIDEmptyIsNoOp(t *testing.T) {
	base := context.Background()
	ctx := WithRequestID(base, "")
	// Empty id must return the original context unchanged (no value stored).
	if ctx != base {
		t.Error("WithRequestID with empty id should return the original context")
	}
	if got := RequestID(ctx); got != "" {
		t.Errorf("RequestID = %q, want empty", got)
	}
}

func TestRequestIDMissing(t *testing.T) {
	if got := RequestID(context.Background()); got != "" {
		t.Errorf("RequestID on bare context = %q, want empty", got)
	}
}

func TestRequestIDWrongType(t *testing.T) {
	// A value stored under a different key type must not be returned: the
	// unexported ctxKey is unforgeable, so a foreign key can never collide.
	type otherKey int
	ctx := context.WithValue(context.Background(), otherKey(0), "not-the-id")
	if got := RequestID(ctx); got != "" {
		t.Errorf("RequestID with foreign key = %q, want empty", got)
	}
}

func TestRequestIDOverwrite(t *testing.T) {
	ctx := WithRequestID(context.Background(), "first")
	ctx = WithRequestID(ctx, "second")
	if got := RequestID(ctx); got != "second" {
		t.Errorf("RequestID after overwrite = %q, want %q", got, "second")
	}
}

func TestRecoverGoroutineCatchesPanic(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)

	// Invoke RecoverGoroutine via a deferred call inside a function that panics.
	// If the panic is not swallowed, this test goroutine would crash the run.
	func() {
		defer RecoverGoroutine("test.loop")
		panic("boom")
	}()

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("log output is not valid JSON: %v\nraw: %s", err, buf.String())
	}
	if lvl, _ := rec["level"].(string); lvl != "ERROR" {
		t.Errorf("log level = %q, want ERROR", lvl)
	}
	if msg, _ := rec["msg"].(string); msg != "goroutine panic" {
		t.Errorf("log msg = %q, want %q", msg, "goroutine panic")
	}
	if g, _ := rec["goroutine"].(string); g != "test.loop" {
		t.Errorf("goroutine attr = %q, want %q", g, "test.loop")
	}
	if p, _ := rec["panic"].(string); p != "boom" {
		t.Errorf("panic attr = %q, want %q", p, "boom")
	}
	if stack, _ := rec["stack"].(string); !strings.Contains(stack, "log_test.go") {
		t.Errorf("stack attr does not reference the panicking site; got: %q", stack)
	}
}

func TestRecoverGoroutineNoPanic(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	// No panic in flight: RecoverGoroutine must be a silent no-op.
	func() {
		defer RecoverGoroutine("test.clean")
	}()

	if buf.Len() != 0 {
		t.Errorf("RecoverGoroutine logged with no panic in flight: %s", buf.String())
	}
}
