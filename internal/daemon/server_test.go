package daemon_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/daemon"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/browsertool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/cdptool"
	"github.com/gorilla/websocket"
)

// fakeBrowser stands up /json/version + a CDP-style WS, with a configurable
// handler. Tracks how many WS dials were performed so tests can assert
// session reuse across daemon calls.
type fakeBrowser struct {
	*httptest.Server
	dials atomic.Int64
}

func newFakeBrowser(t *testing.T, handle func(*testing.T, *websocket.Conn, map[string]any)) *fakeBrowser {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	var server *httptest.Server
	fb := &fakeBrowser{}
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Browser":              "FakeBrowser/1.0",
			"webSocketDebuggerUrl": wsURL,
		})
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		fb.dials.Add(1)
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
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
			handle(t, ws, f)
		}
	})
	server = httptest.NewServer(mux)
	fb.Server = server
	return fb
}

func writeFrame(t *testing.T, ws *websocket.Conn, v any) {
	t.Helper()
	raw, _ := json.Marshal(v)
	_ = ws.WriteMessage(websocket.TextMessage, raw)
}

func defaultRegistry() *tools.Registry {
	r := tools.NewRegistry()
	cdptool.Register(r)
	browsertool.Register(r)
	return r
}

func TestDaemon_HealthAndAuth(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "daemon.json")
	srv, err := daemon.Start(context.Background(), defaultRegistry(), daemon.Config{
		Port:       0,
		SocketPath: socket,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer srv.Stop(context.Background())

	_ = srv.Addr() // address is also recorded in the socket file

	info, err := daemon.ReadSocketFile(socket)
	if err != nil {
		t.Fatalf("read socket: %v", err)
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", info.Port)

	// /v1/health is auth-free.
	resp, err := http.Get(base + "/v1/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("health status=%d", resp.StatusCode)
	}

	// /v1/list-tools requires auth — bare GET is rejected.
	resp, err = http.Get(base + "/v1/list-tools")
	if err != nil {
		t.Fatalf("list-tools no auth: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	// With token: 200.
	req, _ := http.NewRequest("GET", base+"/v1/list-tools", nil)
	req.Header.Set("Authorization", "Bearer "+info.Token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list-tools with auth: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestDaemon_TenCallsReuseOneConnection(t *testing.T) {
	// Phase 5 deliverable: ten sequential `call` invocations against a
	// running daemon reuse one CDP connection. The fake browser counts WS
	// upgrade attempts; we assert exactly 1 across 10 calls.
	fb := newFakeBrowser(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		method, _ := f["method"].(string)
		switch method {
		case "Target.getTargets":
			writeFrame(t, ws, map[string]any{
				"id": f["id"],
				"result": map[string]any{
					"targetInfos": []any{
						map[string]any{
							"targetId": "T1",
							"type":     "page",
							"url":      "https://app/main",
						},
					},
				},
			})
		case "Target.attachToTarget":
			writeFrame(t, ws, map[string]any{
				"id":     f["id"],
				"result": map[string]any{"sessionId": "cdp-1"},
			})
		case "Runtime.evaluate":
			writeFrame(t, ws, map[string]any{
				"id": f["id"],
				"result": map[string]any{
					"result": map[string]any{"type": "number", "value": 2},
				},
			})
		}
	})
	defer fb.Close()

	socket := filepath.Join(t.TempDir(), "daemon.json")
	srv, err := daemon.Start(context.Background(), defaultRegistry(), daemon.Config{
		Port:       0,
		SocketPath: socket,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer srv.Stop(context.Background())

	info, err := daemon.ReadSocketFile(socket)
	if err != nil {
		t.Fatalf("read socket: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type diag struct {
		CDPRoundTrips int64 `json:"cdpRoundTrips"`
	}

	var firstRoundTrips, lastRoundTrips int64
	for i := 0; i < 10; i++ {
		req := daemon.CallRequest{
			Tool:   "cdp.evaluate",
			Params: json.RawMessage(`{"expression":"1+1"}`),
			Endpoint: daemon.EndpointConfig{
				BrowserURL: fb.URL,
			},
			SessionID: "default",
		}
		env, err := daemon.CallDaemon(ctx, info, req)
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if !env.OK {
			t.Fatalf("call %d failed: %+v", i, env.Error)
		}
		// Re-encode env.Diagnostics into our typed struct.
		raw, _ := json.Marshal(env.Diagnostics)
		var d diag
		_ = json.Unmarshal(raw, &d)
		if i == 0 {
			firstRoundTrips = d.CDPRoundTrips
		}
		lastRoundTrips = d.CDPRoundTrips
	}

	dials := fb.dials.Load()
	if dials != 1 {
		t.Errorf("expected 1 WS dial across 10 calls, got %d", dials)
	}
	// First call pays getTargets+attachToTarget+evaluate (3 roundtrips);
	// subsequent calls hit the selection cache and only pay evaluate (1).
	if firstRoundTrips < 2 {
		t.Errorf("first call cdpRoundTrips=%d, want >= 2 (getTargets+attach+eval)", firstRoundTrips)
	}
	if lastRoundTrips != 1 {
		t.Errorf("steady-state cdpRoundTrips=%d, want 1 (evaluate only)", lastRoundTrips)
	}
}

func TestDaemon_StopRemovesSocketFile(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "daemon.json")
	srv, err := daemon.Start(context.Background(), defaultRegistry(), daemon.Config{
		Port:       0,
		SocketPath: socket,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := daemon.ReadSocketFile(socket); err != nil {
		t.Fatalf("expected socket file: %v", err)
	}
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := daemon.ReadSocketFile(socket); err == nil {
		t.Fatal("expected socket file to be removed on Stop")
	}
}
