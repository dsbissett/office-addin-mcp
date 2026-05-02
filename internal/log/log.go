// Package log carries a request-scoped correlation id on context.Context plus a
// shared panic-recovery helper. It is a leaf package — only stdlib imports —
// so any layer (cdp, session, tools, mcp adapter) can pick the id off context
// without inviting a circular dependency.
package log

import (
	"context"
	"log/slog"
	"runtime/debug"
)

type ctxKey int

const requestIDKey ctxKey = 0

// WithRequestID stores id on ctx so downstream layers can correlate logs and
// envelope diagnostics. The empty string is a no-op.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID returns the id stored by WithRequestID, or "" if none.
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// RecoverGoroutine is a defer-friendly panic catcher for long-lived goroutines
// (read loops, GC tickers, output pumps). It logs the panic + stack at error
// level and swallows the value so the surrounding code (which usually owns a
// connection lifecycle) can decide what to do next via its existing exit path.
//
//	go func() {
//	    defer log.RecoverGoroutine("cdp.readLoop")
//	    ...
//	}()
func RecoverGoroutine(name string) {
	if r := recover(); r != nil {
		slog.Error("goroutine panic",
			"goroutine", name,
			"panic", r,
			"stack", string(debug.Stack()),
		)
	}
}
