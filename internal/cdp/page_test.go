package cdp

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestPageNavigate_SuccessAndErrorText(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		params, _ := f["params"].(map[string]any)
		url, _ := params["url"].(string)
		result := map[string]any{"frameId": "F1", "loaderId": "L1"}
		if url == "" {
			result = map[string]any{"frameId": "F1", "errorText": "ERR_INVALID_URL"}
		}
		writeJSON(t, ws, map[string]any{"id": f["id"], "result": result})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	res, err := conn.PageNavigate(ctx, "sess-1", "https://example.com")
	if err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if res.FrameID != "F1" || res.LoaderID != "L1" {
		t.Errorf("unexpected result %+v", res)
	}

	// Empty URL should be rejected client-side.
	if _, err := conn.PageNavigate(ctx, "sess-1", ""); err == nil {
		t.Error("expected error for empty url")
	}

	// Empty session should be rejected client-side.
	if _, err := conn.PageNavigate(ctx, "", "https://example.com"); err == nil {
		t.Error("expected error for empty sessionID")
	}
}
