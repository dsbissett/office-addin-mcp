package cdp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeCDP is an in-process CDP-style WebSocket server. The handler is supplied
// per test and gets the full inbound frame so it can echo the matching id.
func fakeCDP(t *testing.T, handler func(t *testing.T, conn *websocket.Conn, frame map[string]any)) (string, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade: %v", err)
			return
		}
		defer ws.Close()
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			var f map[string]any
			if err := json.Unmarshal(data, &f); err != nil {
				continue
			}
			handler(t, ws, f)
		}
	}))
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	return wsURL, srv.Close
}

func writeJSON(t *testing.T, ws *websocket.Conn, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := ws.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestSendCorrelatesIDs(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		id := f["id"]
		method, _ := f["method"].(string)
		writeJSON(t, ws, map[string]any{
			"id":     id,
			"result": map[string]any{"echo": method},
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

	// Issue concurrent sends — ensure each gets its own response.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			method := "Test.method" + string(rune('A'+i))
			raw, err := conn.Send(ctx, "", method, nil)
			if err != nil {
				t.Errorf("send %s: %v", method, err)
				return
			}
			var got struct {
				Echo string `json:"echo"`
			}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Errorf("decode: %v", err)
				return
			}
			if got.Echo != method {
				t.Errorf("got echo %q, want %q", got.Echo, method)
			}
		}(i)
	}
	wg.Wait()
}

func TestSendReturnsRemoteError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{
			"id": f["id"],
			"error": map[string]any{
				"code":    -32601,
				"message": "Method not found",
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

	_, err = conn.Send(ctx, "", "Bogus.method", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var rerr *RemoteError
	if !errors.As(err, &rerr) {
		t.Fatalf("expected *RemoteError, got %T: %v", err, err)
	}
	if rerr.Code != -32601 {
		t.Errorf("got code %d, want -32601", rerr.Code)
	}
}

func TestSendTimeoutRespectsContext(t *testing.T) {
	// Server reads but never replies.
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {})
	defer stop()

	dialCtx, dialCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dialCancel()

	conn, err := Dial(dialCtx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = conn.Send(ctx, "", "Test.never", nil)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if elapsed > time.Second {
		t.Errorf("send took %v, want <1s", elapsed)
	}
}

func TestSendAfterCloseFails(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
	<-conn.Done()

	_, err = conn.Send(ctx, "", "Test.x", nil)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestEventsDispatchedToSubscribers(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		// On any inbound message, push a synthetic event back.
		writeJSON(t, ws, map[string]any{
			"method": "Runtime.executionContextCreated",
			"params": map[string]any{"context": map[string]any{"id": 7}},
		})
		writeJSON(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{},
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

	ch, unsub := conn.Subscribe("Runtime.executionContextCreated", 4)
	defer unsub()

	if _, err := conn.Send(ctx, "", "Trigger.event", nil); err != nil {
		t.Fatalf("send: %v", err)
	}

	select {
	case ev := <-ch:
		if ev.Method != "Runtime.executionContextCreated" {
			t.Errorf("got method %q", ev.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("event not received")
	}
}
