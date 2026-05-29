package webview2_test

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// serveOn9222 binds an in-process HTTP server to the conventional default
// DevTools port (127.0.0.1:9222) that DefaultBrowserURL hardcodes. It serves
// /json/version returning the given webSocketDebuggerUrl. This is a hermetic
// loopback server, not a live Office/WebView2 host. The caller must call the
// returned stop func before any test that expects the default probe to fail.
func serveOn9222(t *testing.T, ws string) (stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:9222")
	if err != nil {
		t.Skipf("cannot bind 127.0.0.1:9222 (something is already listening): %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"` + ws + `"}`))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second}
	go func() { _ = srv.Serve(ln) }()

	// Wait until the listener actually answers so the subsequent Discover probe
	// is deterministic rather than racing server startup.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, derr := net.DialTimeout("tcp", "127.0.0.1:9222", 100*time.Millisecond)
		if derr == nil {
			_ = c.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}

// TestDiscover_DefaultPortProbed exercises priority rung 3: with no explicit
// endpoint configured, Discover probes DefaultBrowserURL (:9222) and, on a
// successful /json/version response, returns SourceDefault.
func TestDiscover_DefaultPortProbed(t *testing.T) {
	stop := serveOn9222(t, "ws://default/zzz")
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ep, err := webview2.Discover(ctx, webview2.Config{})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if ep.Source != webview2.SourceDefault {
		t.Errorf("got source %q, want %q", ep.Source, webview2.SourceDefault)
	}
	if ep.WSURL != "ws://default/zzz" {
		t.Errorf("got ws %q, want ws://default/zzz", ep.WSURL)
	}
	if ep.BrowserURL != webview2.DefaultBrowserURL {
		t.Errorf("got browser %q, want %q", ep.BrowserURL, webview2.DefaultBrowserURL)
	}
}

// TestDiscover_EmptyConfigFallsThroughToNotFound exercises the bottom of the
// priority ladder: nothing configured, default :9222 probe fails (nothing
// listening in the test environment), the OS scan finds no remote-debugging
// port, so Discover returns ErrNotFound. If a real WebView2 with a debugging
// port happens to be running on the test box the scan could succeed, so the
// assertion tolerates that and only fails on an unexpected error shape.
func TestDiscover_EmptyConfigFallsThroughToNotFound(t *testing.T) {
	if c, derr := net.DialTimeout("tcp", "127.0.0.1:9222", 200*time.Millisecond); derr == nil {
		_ = c.Close()
		t.Skip("something is listening on :9222; cannot assert the not-found fall-through")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ep, err := webview2.Discover(ctx, webview2.Config{})
	if err != nil {
		// Expected path in a clean environment: no endpoint anywhere.
		if err != webview2.ErrNotFound {
			t.Fatalf("got error %v, want ErrNotFound", err)
		}
		if ep != (webview2.Endpoint{}) {
			t.Errorf("expected zero Endpoint on error, got %+v", ep)
		}
		return
	}
	// Tolerated path: a real debugging-enabled WebView2/Chrome is running, so
	// the OS scan succeeded. Source must then be the scan source.
	if ep.Source != webview2.SourceScan && ep.Source != webview2.SourceDefault {
		t.Errorf("unexpected success source %q", ep.Source)
	}
}

// TestDiscover_BrowserURLUnparseable confirms the explicit-BrowserURL rung is a
// hard failure when the URL cannot even be parsed/probed, and that the error is
// wrapped with the probe context.
func TestDiscover_BrowserURLUnparseable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := webview2.Discover(ctx, webview2.Config{BrowserURL: "http://127.0.0.1:9/"})
	if err == nil {
		t.Fatal("expected error for unreachable browser URL")
	}
	if !strings.Contains(err.Error(), "probe") {
		t.Errorf("expected wrapped probe error, got %v", err)
	}
	if !strings.Contains(err.Error(), "127.0.0.1:9") {
		t.Errorf("expected error to name the browser URL, got %v", err)
	}
}
