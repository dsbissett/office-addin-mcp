package webview2_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

func newProbeServer(t *testing.T, ws string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"` + ws + `"}`))
	}))
}

func TestDiscover_WSEndpointTakesPriority(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ep, err := webview2.Discover(ctx, webview2.Config{
		WSEndpoint: "ws://127.0.0.1:1234/foo",
		BrowserURL: "http://nope.invalid",
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if ep.Source != webview2.SourceWSEndpoint {
		t.Errorf("got source %q, want %q", ep.Source, webview2.SourceWSEndpoint)
	}
	if ep.WSURL != "ws://127.0.0.1:1234/foo" {
		t.Errorf("got ws %q", ep.WSURL)
	}
}

func TestDiscover_BrowserURLProbed(t *testing.T) {
	srv := newProbeServer(t, "ws://probed/abc")
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ep, err := webview2.Discover(ctx, webview2.Config{BrowserURL: srv.URL})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if ep.Source != webview2.SourceBrowserURL {
		t.Errorf("got source %q, want %q", ep.Source, webview2.SourceBrowserURL)
	}
	if ep.WSURL != "ws://probed/abc" {
		t.Errorf("got ws %q, want ws://probed/abc", ep.WSURL)
	}
	if ep.BrowserURL != srv.URL {
		t.Errorf("got browser %q, want %q", ep.BrowserURL, srv.URL)
	}
}

func TestDiscover_BrowserURLHardFailOnMiss(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := webview2.Discover(ctx, webview2.Config{
		BrowserURL: "http://127.0.0.1:1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "probe") {
		t.Errorf("expected probe error, got %v", err)
	}
}
