package session

import (
	"context"
	"testing"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// BenchmarkSessionAcquireWarm measures the steady-state Acquire fast path:
// connMu.RLock + endpointEqual + connDone-select + return. No dial, no
// state mutation. This is what a long-running daemon hits on every tool
// call after the first, so a regression here scales linearly with traffic.
func BenchmarkSessionAcquireWarm(b *testing.B) {
	fb := newFakeBrowser(b)
	defer fb.Close()
	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	defer s.Close()
	ep := webview2.Config{BrowserURL: fb.URL}

	// Prime the connection so subsequent Acquires take the fast path.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, rel, err := s.Acquire(ctx, ep); err != nil {
		b.Fatalf("prime: %v", err)
	} else {
		rel()
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, release, err := s.Acquire(ctx, ep)
		if err != nil {
			b.Fatalf("acquire: %v", err)
		}
		release()
	}
}

// BenchmarkSessionParallelAcquire validates the F5 contract under
// b.RunParallel: many goroutines hammering the same Session must not
// serialize, so wall-clock time / op should drop ~linearly with GOMAXPROCS
// up to the read-lock contention floor. Pair this with
// `go test -bench=ParallelAcquire -cpu=1,2,4,8` to chart scaling.
func BenchmarkSessionParallelAcquire(b *testing.B) {
	fb := newFakeBrowser(b)
	defer fb.Close()
	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	defer s.Close()
	ep := webview2.Config{BrowserURL: fb.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, rel, err := s.Acquire(ctx, ep); err != nil {
		b.Fatalf("prime: %v", err)
	} else {
		rel()
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, release, err := s.Acquire(ctx, ep)
			if err != nil {
				b.Errorf("acquire: %v", err)
				return
			}
			release()
		}
	})
}

// BenchmarkSelectionCacheHit isolates the per-call cost of a sticky
// selector lookup — the hot path inside RunEnv.Attach when an agent
// hammers the same target. Just stateMu lock + matches() + return.
func BenchmarkSelectionCacheHit(b *testing.B) {
	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	s.SetSelected("", "main", cdp.TargetInfo{TargetID: "T1", Type: "page"}, "cdp-1")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := s.Selected("", "main"); !ok {
			b.Fatal("selection miss")
		}
	}
}

// BenchmarkEnsureEnabledHit measures the cheap path of EnsureEnabled
// where the (cdpSessionID, domain) pair is already marked. This dominates
// dispatcher cost for any auto-enable domain after the first hit per
// connection, so it stays cheap (single stateMu.Lock + nested map lookup).
func BenchmarkEnsureEnabledHit(b *testing.B) {
	s := &Session{id: "default", cfg: Config{}.withDefaults()}
	send := &fakeSender{}
	ctx := context.Background()
	if err := s.EnsureEnabled(ctx, send, "cdp-1", "Page"); err != nil {
		b.Fatalf("prime: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.EnsureEnabled(ctx, send, "cdp-1", "Page"); err != nil {
			b.Fatalf("ensure: %v", err)
		}
	}
}
