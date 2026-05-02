package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
	"github.com/gorilla/websocket"
)

// fakeBrowser stands up a /json/version endpoint plus a CDP-style WS at /ws.
// dialCount counts how many times /ws was upgraded — useful for asserting
// that a reused session does NOT redial.
type fakeBrowser struct {
	*httptest.Server
	dialCount atomic.Int64
}

func newFakeBrowser(t *testing.T) *fakeBrowser {
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
		fb.dialCount.Add(1)
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				return
			}
			// We don't care about the messages here — sessions tests focus
			// on connection lifecycle, not protocol behavior.
		}
	})
	server = httptest.NewServer(mux)
	fb.Server = server
	return fb
}

func TestSession_AcquireDialsOnceAndReuses(t *testing.T) {
	fb := newFakeBrowser(t)
	defer fb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	defer s.Close()

	ep := webview2.Config{BrowserURL: fb.URL}

	for i := 0; i < 5; i++ {
		conn, release, err := s.Acquire(ctx, ep)
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		if conn == nil {
			t.Fatalf("conn nil")
		}
		release()
	}
	if got := fb.dialCount.Load(); got != 1 {
		t.Errorf("expected 1 dial, got %d", got)
	}
}

func TestSession_ReconnectBudgetExhaustion(t *testing.T) {
	// Point at an unreachable endpoint so every dial fails — each Acquire
	// records a reconnect attempt; the 4th must be rejected.
	s := &Session{id: "default", cfg: Config{
		ReconnectMax:    3,
		ReconnectWindow: 60 * time.Second,
	}}
	ep := webview2.Config{BrowserURL: "http://127.0.0.1:1"}

	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		_, _, err := s.Acquire(ctx, ep)
		cancel()
		if err == nil {
			t.Fatalf("expected dial failure on attempt %d", i)
		}
		if !errors.Is(err, ErrDialFailed) {
			t.Errorf("attempt %d: expected ErrDialFailed, got %v", i, err)
		}
	}

	// Fourth attempt: budget exhausted before we even try to dial.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, _, err := s.Acquire(ctx, ep)
	if err == nil || !errors.Is(err, ErrReconnectBudgetExhausted) {
		t.Fatalf("expected ErrReconnectBudgetExhausted, got %v", err)
	}
}

func TestSession_StickySelectionCache(t *testing.T) {
	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	target := cdp.TargetInfo{TargetID: "T1", Type: "page", URL: "https://app/main"}

	if _, ok := s.Selected("", ""); ok {
		t.Fatal("empty selection should miss")
	}

	s.SetSelected("", "main", target, "cdp-1")
	got, ok := s.Selected("", "main")
	if !ok {
		t.Fatal("expected cache hit on matching selector")
	}
	if got.Target.TargetID != "T1" || got.SessionID != "cdp-1" {
		t.Errorf("got %+v", got)
	}

	if _, ok := s.Selected("", "different"); ok {
		t.Error("different selector should miss")
	}
	if _, ok := s.Selected("T1", ""); ok {
		t.Error("targetId selector should miss when only urlPattern was cached")
	}

	s.InvalidateSelection()
	if _, ok := s.Selected("", "main"); ok {
		t.Error("invalidate should clear cache")
	}
}

func TestSession_ReconnectClearsSelectionCache(t *testing.T) {
	fb := newFakeBrowser(t)
	defer fb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	defer s.Close()

	ep := webview2.Config{BrowserURL: fb.URL}

	conn, release, err := s.Acquire(ctx, ep)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	s.SetSelected("", "x", cdp.TargetInfo{TargetID: "T1"}, "cdp-1")
	release()

	// Force a reconnect by closing the conn under the session.
	conn.Close()
	<-conn.Done()

	conn2, release2, err := s.Acquire(ctx, ep)
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	defer release2()
	if conn2 == conn {
		t.Error("expected fresh connection after Done")
	}
	if _, ok := s.Selected("", "x"); ok {
		t.Error("expected selection cache to be cleared on reconnect")
	}
}

func TestManager_GetIsStable(t *testing.T) {
	m := NewManager(Config{})
	defer m.Close()

	a := m.Get("alpha")
	b := m.Get("alpha")
	if a != b {
		t.Error("Get should return same session for same id")
	}
	if d := m.Get(""); d != m.Get("default") {
		t.Error("empty id should resolve to 'default'")
	}
}

func TestManager_IdleGC(t *testing.T) {
	m := NewManager(Config{IdleTimeout: 200 * time.Millisecond})
	defer m.Close()

	fb := newFakeBrowser(t)
	defer fb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s := m.Get("temp")
	_, release, err := s.Acquire(ctx, webview2.Config{BrowserURL: fb.URL})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	release()

	// Wait long enough for at least one GC tick (interval is IdleTimeout/4).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ids := m.Snapshot()
		if len(ids) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected idle session to be GC'd, still present: %v", m.Snapshot())
}

func TestManager_DropClosesSession(t *testing.T) {
	m := NewManager(Config{})
	defer m.Close()

	fb := newFakeBrowser(t)
	defer fb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s := m.Get("x")
	conn, release, err := s.Acquire(ctx, webview2.Config{BrowserURL: fb.URL})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	release()

	m.Drop("x")

	select {
	case <-conn.Done():
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("expected Drop to close the underlying connection")
	}
}

func TestSession_DefaultSelection(t *testing.T) {
	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	if _, ok := s.DefaultSelection(); ok {
		t.Fatal("default selection should be empty initially")
	}
	target := cdp.TargetInfo{TargetID: "T1", Type: "page", URL: "https://app/"}
	s.SetDefaultSelection(target, "cdp-1")
	got, ok := s.DefaultSelection()
	if !ok {
		t.Fatal("expected default selection after SetDefaultSelection")
	}
	if got.Target.TargetID != "T1" || got.SessionID != "cdp-1" {
		t.Errorf("got %+v", got)
	}
	s.ClearDefaultSelection()
	if _, ok := s.DefaultSelection(); ok {
		t.Error("ClearDefaultSelection should reset")
	}
}

func TestSession_SnapshotCache(t *testing.T) {
	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	if s.Snapshot() != nil {
		t.Fatal("snapshot should start nil")
	}
	snap := &Snapshot{
		TargetID:     "T1",
		CDPSessionID: "S1",
		Nodes:        map[string]SnapshotNode{"uid-1": {UID: "uid-1", BackendNodeID: 7}},
	}
	s.SetSnapshot(snap)
	got := s.Snapshot()
	if got == nil || got.Nodes["uid-1"].BackendNodeID != 7 {
		t.Fatalf("snapshot lookup failed: %+v", got)
	}
}
