package cdp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolveBrowserWSURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"Browser": "Chrome/127",
			"webSocketDebuggerUrl": "ws://127.0.0.1:9222/devtools/browser/abc"
		}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := ResolveBrowserWSURL(ctx, srv.URL)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := "ws://127.0.0.1:9222/devtools/browser/abc"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveBrowserWSURL_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ResolveBrowserWSURL(ctx, srv.URL)
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("expected status 500 error, got %v", err)
	}
}
