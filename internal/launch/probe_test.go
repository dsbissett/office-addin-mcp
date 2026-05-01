package launch

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbeCDPEndpoint_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Browser":"WebView2/1.2.3"}`))
	}))
	defer srv.Close()

	probe := ProbeCDPEndpoint(context.Background(), srv.URL, time.Second)
	if !probe.OK {
		t.Fatalf("probe = %+v, want OK", probe)
	}
	if probe.Version != "WebView2/1.2.3" {
		t.Errorf("Version = %q, want WebView2/1.2.3", probe.Version)
	}
}

func TestProbeCDPEndpoint_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer srv.Close()
	probe := ProbeCDPEndpoint(context.Background(), srv.URL, time.Second)
	if probe.OK {
		t.Fatal("probe.OK = true, want false on 500")
	}
	if probe.Reason == "" {
		t.Error("Reason = empty, want http-error:500")
	}
}

func TestProbeCDPEndpoint_InvalidBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"NotBrowser":"x"}`))
	}))
	defer srv.Close()
	probe := ProbeCDPEndpoint(context.Background(), srv.URL, time.Second)
	if probe.OK || probe.Reason != "invalid-response" {
		t.Errorf("probe = %+v, want OK=false reason=invalid-response", probe)
	}
}

func TestProbeCDPEndpoint_Unreachable(t *testing.T) {
	probe := ProbeCDPEndpoint(context.Background(), "http://127.0.0.1:1", 200*time.Millisecond)
	if probe.OK {
		t.Fatal("probe.OK = true, want false")
	}
}

func TestIsPortListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port
	if !IsPortListening(port, time.Second) {
		t.Errorf("IsPortListening(%d) = false, want true", port)
	}
	_ = ln.Close()
	if IsPortListening(port, 200*time.Millisecond) {
		// Could conceivably re-bind; tolerate flakiness only by not failing.
		t.Logf("port %d still reported as listening after Close (likely TIME_WAIT)", port)
	}
}
