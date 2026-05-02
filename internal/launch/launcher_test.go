package launch

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestLaunchIfNeeded_Preexisting verifies that when something is already
// serving /json/version on the requested port, LaunchIfNeeded returns
// "preexisting" without trying to spawn office-addin-debugging — the whole
// point of the helper. We exercise this with a tiny stub HTTP server bound
// to 127.0.0.1 on a chosen port so we can pass that port to LaunchIfNeeded
// (which probes localhost only, never a custom URL).
func TestLaunchIfNeeded_Preexisting(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Browser":"LaunchIfNeededTestStub/1.0"}`))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	})

	res, source, err := LaunchIfNeeded(context.Background(), nil, LaunchOptions{Port: port})
	if err != nil {
		t.Fatalf("LaunchIfNeeded: %v", err)
	}
	if source != "preexisting" {
		t.Errorf("source = %q, want preexisting", source)
	}
	wantURL := fmt.Sprintf("http://localhost:%d", port)
	if res == nil || res.CDPURL != wantURL {
		t.Errorf("CDPURL = %q, want %q", res.CDPURL, wantURL)
	}
}

// TestLaunchIfNeeded_NoProjectErrors verifies that when nothing's listening
// AND the caller didn't supply a project, LaunchIfNeeded returns a structured
// LaunchError rather than calling LaunchExcel (which would fail on a
// nil-deref).
func TestLaunchIfNeeded_NoProjectErrors(t *testing.T) {
	// Use port 1 — guaranteed not to have a CDP server bound.
	_, _, err := LaunchIfNeeded(context.Background(), nil, LaunchOptions{Port: 1})
	if err == nil {
		t.Fatal("LaunchIfNeeded returned nil error, want LaunchError")
	}
	le := AsLaunchError(err)
	if le == nil {
		t.Fatalf("err = %v, want *LaunchError", err)
	}
	if !strings.Contains(le.Message, "no add-in project supplied") {
		t.Errorf("Message = %q, want substring 'no add-in project supplied'", le.Message)
	}
}
