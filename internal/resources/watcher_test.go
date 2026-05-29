package resources

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// fpProvider builds a Provider whose *.discover stub returns a fingerprint
// produced by fp() on every call, letting a test drive change detection. When
// fp returns ("", false) the discover dispatch fails so Fingerprint errors.
func fpProvider(t *testing.T, fp func() (string, bool)) *Provider {
	t.Helper()
	reg := tools.NewRegistry()
	schema := json.RawMessage(`{"type":"object"}`)
	for _, name := range []string{"excel.discover", "word.discover", "outlook.discover", "powerpoint.discover", "onenote.discover"} {
		reg.MustRegister(tools.Tool{
			Name:      name,
			Schema:    schema,
			NoSession: true,
			Run: func(_ context.Context, _ json.RawMessage, _ *tools.RunEnv) tools.Result {
				val, ok := fp()
				if !ok {
					return tools.Fail(tools.CategoryConnection, "session_dial_failed", "no host", true)
				}
				return tools.OK(map[string]any{"fingerprint": val})
			},
		})
	}
	disp := &tools.Dispatcher{Registry: reg}
	return &Provider{
		Disp:     disp,
		Endpoint: func() webview2.Config { return webview2.Config{BrowserURL: "http://127.0.0.1:9222"} },
	}
}

func TestNewWatcher_Defaults(t *testing.T) {
	p := fpProvider(t, func() (string, bool) { return "fp1", true })
	w := NewWatcher(p, nil)
	if w.PollInterval != 30*time.Second {
		t.Errorf("PollInterval = %v, want 30s", w.PollInterval)
	}
	if w.subs == nil {
		t.Error("subs map not initialized")
	}
}

func TestWatcher_SubscribeAndUnsubscribe(t *testing.T) {
	p := fpProvider(t, func() (string, bool) { return "stable", true })
	w := NewWatcher(p, nil)
	// Slow poll so the background goroutine never actually fires during the test.
	w.PollInterval = time.Hour

	if err := w.Subscribe(context.Background(), "office://excel/Book1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	w.mu.RLock()
	sub, ok := w.subs["office://excel/Book1"]
	w.mu.RUnlock()
	if !ok {
		t.Fatal("subscription not recorded")
	}
	if sub.fingerprint != "stable" {
		t.Errorf("initial fingerprint = %q, want stable", sub.fingerprint)
	}

	w.Unsubscribe("office://excel/Book1")
	w.mu.RLock()
	_, ok = w.subs["office://excel/Book1"]
	w.mu.RUnlock()
	if ok {
		t.Error("subscription still present after Unsubscribe")
	}
}

func TestWatcher_SubscribeDuplicateUpdatesFingerprint(t *testing.T) {
	// The first fingerprint is "a"; the second Subscribe sees "b" and should
	// update the existing subscription in place rather than start a new poll.
	var n atomic.Int32
	p := fpProvider(t, func() (string, bool) {
		if n.Add(1) == 1 {
			return "a", true
		}
		return "b", true
	})
	w := NewWatcher(p, nil)
	w.PollInterval = time.Hour

	if err := w.Subscribe(context.Background(), "office://excel/Book1"); err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	if err := w.Subscribe(context.Background(), "office://excel/Book1"); err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}

	w.mu.RLock()
	sub := w.subs["office://excel/Book1"]
	count := len(w.subs)
	fp := sub.fingerprint
	w.mu.RUnlock()
	if count != 1 {
		t.Errorf("subs count = %d, want 1", count)
	}
	if fp != "b" {
		t.Errorf("fingerprint = %q, want updated to b", fp)
	}
	w.Close()
}

func TestWatcher_SubscribeFingerprintError(t *testing.T) {
	p := fpProvider(t, func() (string, bool) { return "", false })
	w := NewWatcher(p, nil)
	if err := w.Subscribe(context.Background(), "office://excel/Book1"); err == nil {
		t.Fatal("want error when initial Fingerprint fails")
	}
	w.mu.RLock()
	count := len(w.subs)
	w.mu.RUnlock()
	if count != 0 {
		t.Errorf("subs count = %d, want 0 after failed Subscribe", count)
	}
}

func TestWatcher_UnsubscribeUnknown(t *testing.T) {
	p := fpProvider(t, func() (string, bool) { return "x", true })
	w := NewWatcher(p, nil)
	// No panic / no block when unsubscribing something never subscribed.
	w.Unsubscribe("office://excel/Nope")
}

func TestWatcher_Close(t *testing.T) {
	p := fpProvider(t, func() (string, bool) { return "x", true })
	w := NewWatcher(p, nil)
	w.PollInterval = time.Hour

	for _, uri := range []string{"office://excel/A", "office://word/B"} {
		if err := w.Subscribe(context.Background(), uri); err != nil {
			t.Fatalf("Subscribe %s: %v", uri, err)
		}
	}
	w.Close()

	w.mu.RLock()
	count := len(w.subs)
	w.mu.RUnlock()
	if count != 0 {
		t.Errorf("subs count = %d after Close, want 0", count)
	}
	// Close on an empty watcher is a no-op.
	w.Close()
}

func TestWatcher_PollDetectsChangeAndNotifies(t *testing.T) {
	// First fingerprint is "v1" (captured at Subscribe). Subsequent poll ticks
	// see "v2", which differs, so notify must fire with the URI.
	var n atomic.Int32
	p := fpProvider(t, func() (string, bool) {
		if n.Add(1) <= 1 {
			return "v1", true
		}
		return "v2", true
	})

	notified := make(chan string, 4)
	w := NewWatcher(p, func(_ context.Context, uri string) {
		notified <- uri
	})
	w.PollInterval = 5 * time.Millisecond

	if err := w.Subscribe(context.Background(), "office://excel/Book1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer w.Close()

	select {
	case got := <-notified:
		if got != "office://excel/Book1" {
			t.Errorf("notify uri = %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected change notification, got none")
	}

	// After the change is recorded, the stored fingerprint should be "v2".
	w.mu.RLock()
	fp := w.subs["office://excel/Book1"].fingerprint
	w.mu.RUnlock()
	if fp != "v2" {
		t.Errorf("stored fingerprint = %q, want v2", fp)
	}
}

func TestWatcher_PollNoChangeNoNotify(t *testing.T) {
	// Fingerprint never changes -> notify must not fire.
	p := fpProvider(t, func() (string, bool) { return "constant", true })
	var notifyCount atomic.Int32
	w := NewWatcher(p, func(context.Context, string) {
		notifyCount.Add(1)
	})
	w.PollInterval = 5 * time.Millisecond

	if err := w.Subscribe(context.Background(), "office://excel/Book1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	// Let several poll ticks elapse.
	time.Sleep(60 * time.Millisecond)
	w.Close()

	if c := notifyCount.Load(); c != 0 {
		t.Errorf("notify fired %d times, want 0 on a stable fingerprint", c)
	}
}

func TestWatcher_PollFingerprintErrorContinues(t *testing.T) {
	// First call (Subscribe) succeeds with "ok"; subsequent poll calls fail.
	// poll must log+continue (no notify, no panic) and keep the subscription.
	var n atomic.Int32
	p := fpProvider(t, func() (string, bool) {
		if n.Add(1) <= 1 {
			return "ok", true
		}
		return "", false
	})
	var notifyCount atomic.Int32
	w := NewWatcher(p, func(context.Context, string) { notifyCount.Add(1) })
	w.PollInterval = 5 * time.Millisecond

	if err := w.Subscribe(context.Background(), "office://excel/Book1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	time.Sleep(60 * time.Millisecond)

	w.mu.RLock()
	_, ok := w.subs["office://excel/Book1"]
	w.mu.RUnlock()
	if !ok {
		t.Error("subscription dropped after a polling fingerprint error")
	}
	w.Close()

	if c := notifyCount.Load(); c != 0 {
		t.Errorf("notify fired %d times despite fingerprint errors", c)
	}
}

func TestWatcher_PollCtxCancelViaUnsubscribe(t *testing.T) {
	// Unsubscribe cancels the poll ctx; the <-sub.done in Unsubscribe proves the
	// goroutine returned via the ctx.Done() branch. Use a long interval so the
	// goroutine is parked on the ticker when cancellation arrives.
	p := fpProvider(t, func() (string, bool) { return "x", true })
	w := NewWatcher(p, nil)
	w.PollInterval = time.Hour

	if err := w.Subscribe(context.Background(), "office://excel/Book1"); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	done := make(chan struct{})
	go func() {
		w.Unsubscribe("office://excel/Book1")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Unsubscribe did not return; poll goroutine likely leaked")
	}
}

// ensure no data race when multiple subscribers exist (run with -race).
func TestWatcher_ConcurrentSubscribeClose(t *testing.T) {
	p := fpProvider(t, func() (string, bool) { return "x", true })
	w := NewWatcher(p, nil)
	w.PollInterval = 10 * time.Millisecond

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uri := "office://excel/Book" + string(rune('A'+i))
			_ = w.Subscribe(context.Background(), uri)
		}(i)
	}
	wg.Wait()
	w.Close()
}
