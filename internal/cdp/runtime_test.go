package cdp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestEvaluateUnwrapsExceptionDetails(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{
			"id": f["id"],
			"result": map[string]any{
				"result": map[string]any{"type": "undefined"},
				"exceptionDetails": map[string]any{
					"exceptionId": 1,
					"text":        "Uncaught",
					"exception": map[string]any{
						"type":        "object",
						"className":   "ReferenceError",
						"description": "ReferenceError: nope is not defined",
					},
				},
			},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	res, err := conn.Evaluate(ctx, "session-1", EvaluateParams{Expression: "nope"})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.ExceptionDetails == nil {
		t.Fatal("expected exceptionDetails")
	}
	if !strings.Contains(res.ExceptionDetails.String(), "ReferenceError") {
		t.Errorf("expected description in String(), got %q", res.ExceptionDetails.String())
	}
}

func TestEvaluateRequiresSessionID(t *testing.T) {
	conn := &Connection{}
	_, err := conn.Evaluate(context.Background(), "", EvaluateParams{Expression: "1"})
	if err == nil {
		t.Fatal("expected error for empty sessionID")
	}
}

func TestEvaluateRoutesSessionID(t *testing.T) {
	var seenSession string
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		if s, ok := f["sessionId"].(string); ok {
			seenSession = s
		}
		writeJSON(t, ws, map[string]any{
			"id": f["id"],
			"result": map[string]any{
				"result": map[string]any{"type": "number", "value": json.RawMessage("2")},
			},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	res, err := conn.Evaluate(ctx, "abc-123", EvaluateParams{Expression: "1+1", ReturnByValue: true})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if seenSession != "abc-123" {
		t.Errorf("server saw sessionId %q, want %q", seenSession, "abc-123")
	}
	if string(res.Result.Value) != "2" {
		t.Errorf("got value %s, want 2", string(res.Result.Value))
	}
}

// keep the linter happy if upstream gorilla/websocket changes
var (
	_ = http.StatusOK
	_ = time.Second
)
