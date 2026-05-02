package session

import (
	"context"
	"testing"
)

// TestEnsureEnabledOnceAcrossCalls confirms that N sequential Page.* calls
// against the same (session, cdpSessionID) only emit "Page.enable" once.
// Mirrors how the dispatcher will use EnsureEnabled in production.
func TestEnsureEnabledOnceAcrossCalls(t *testing.T) {
	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	send := &fakeSender{}
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := s.EnsureEnabled(ctx, send, "cdp-1", "Page"); err != nil {
			t.Fatalf("ensure %d: %v", i, err)
		}
	}
	if got := send.methodCount("Page.enable"); got != 1 {
		t.Errorf("Page.enable invoked %d times across 5 EnsureEnabled calls, want 1", got)
	}
	if !s.IsEnabled("cdp-1", "Page") {
		t.Error("expected (cdp-1, Page) marked enabled")
	}
}

// TestEnsureEnabledPerCDPSession confirms each (cdpSessionID, domain) pair
// is tracked independently — re-attaching to a different target re-enables.
func TestEnsureEnabledPerCDPSession(t *testing.T) {
	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	send := &fakeSender{}
	ctx := context.Background()

	for _, sid := range []string{"cdp-A", "cdp-B", "cdp-A", "cdp-B"} {
		if err := s.EnsureEnabled(ctx, send, sid, "Runtime"); err != nil {
			t.Fatalf("ensure: %v", err)
		}
	}
	if got := send.methodCount("Runtime.enable"); got != 2 {
		t.Errorf("Runtime.enable invoked %d times for 2 distinct CDP sessions, want 2", got)
	}
}

// TestEnsureEnabledClearedOnReconnect confirms dropConnLocked wipes the
// enabled bookkeeping — Chrome resets domain state across connections, so
// the next EnsureEnabled must re-issue the .enable call.
func TestEnsureEnabledClearedOnReconnect(t *testing.T) {
	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	send := &fakeSender{}
	ctx := context.Background()

	if err := s.EnsureEnabled(ctx, send, "cdp-1", "Page"); err != nil {
		t.Fatalf("first enable: %v", err)
	}
	if !s.IsEnabled("cdp-1", "Page") {
		t.Fatal("expected enabled before reconnect")
	}

	// Simulate the conn-loss path that dispatcher.Acquire takes when the
	// underlying socket dies. dropConnLocked is the canonical clearing point;
	// it must be called with connMu write-locked.
	s.connMu.Lock()
	s.dropConnLocked()
	s.connMu.Unlock()

	if s.IsEnabled("cdp-1", "Page") {
		t.Error("expected enabled bookkeeping wiped after dropConnLocked")
	}
	if err := s.EnsureEnabled(ctx, send, "cdp-1", "Page"); err != nil {
		t.Fatalf("post-reconnect enable: %v", err)
	}
	if got := send.methodCount("Page.enable"); got != 2 {
		t.Errorf("Page.enable invoked %d times across reconnect, want 2", got)
	}
}
