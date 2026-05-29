package cdp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// --- RemoteError.Error ---------------------------------------------------

func TestRemoteErrorErrorFormatting(t *testing.T) {
	withData := &RemoteError{Code: -32000, Message: "boom", Data: "extra detail"}
	got := withData.Error()
	if !strings.Contains(got, "boom") || !strings.Contains(got, "-32000") || !strings.Contains(got, "extra detail") {
		t.Errorf("Error() with data = %q, missing expected parts", got)
	}

	noData := &RemoteError{Code: -32601, Message: "Method not found"}
	got = noData.Error()
	if !strings.Contains(got, "Method not found") || !strings.Contains(got, "-32601") {
		t.Errorf("Error() without data = %q, missing expected parts", got)
	}
	if strings.Contains(got, ":") && strings.Count(got, ":") > 1 {
		// "cdp: Method not found (code -32601)" has exactly one colon segment;
		// the data branch would add another ": data". Guard against accidental
		// data inclusion.
		t.Errorf("Error() without data leaked a data segment: %q", got)
	}
}

// --- RoundTrips ----------------------------------------------------------

func TestRoundTripsCounts(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{"id": f["id"], "result": map[string]any{}})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if got := conn.RoundTrips(); got != 0 {
		t.Errorf("RoundTrips before any send = %d, want 0", got)
	}
	for i := 0; i < 3; i++ {
		if _, err := conn.Send(ctx, "", "Test.method", nil); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if got := conn.RoundTrips(); got != 3 {
		t.Errorf("RoundTrips after 3 sends = %d, want 3", got)
	}
}

// --- GetTargets ----------------------------------------------------------

func TestGetTargetsDecodes(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{
			"id": f["id"],
			"result": map[string]any{
				"targetInfos": []map[string]any{
					{"targetId": "t1", "type": "page", "title": "App", "url": "https://app.example/", "attached": true},
					{"targetId": "t2", "type": "service_worker", "url": "https://app.example/sw.js"},
				},
			},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	targets, err := conn.GetTargets(ctx)
	if err != nil {
		t.Fatalf("getTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("got %d targets, want 2", len(targets))
	}
	if targets[0].TargetID != "t1" || targets[0].Type != "page" || !targets[0].Attached {
		t.Errorf("unexpected first target: %+v", targets[0])
	}
}

func TestGetTargetsSendError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{
			"id":    f["id"],
			"error": map[string]any{"code": -32000, "message": "nope"},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.GetTargets(ctx); err == nil {
		t.Fatal("expected error from GetTargets when Send fails")
	}
}

func TestGetTargetsDecodeError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		// targetInfos as a string -> unmarshal into []TargetInfo fails.
		writeJSON(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"targetInfos": "not-an-array"},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, err = conn.GetTargets(ctx)
	if err == nil || !strings.Contains(err.Error(), "decode getTargets") {
		t.Fatalf("expected decode getTargets error, got %v", err)
	}
}

// --- AttachToTarget ------------------------------------------------------

func TestAttachToTargetReturnsSessionID(t *testing.T) {
	var seenParams map[string]any
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		seenParams, _ = f["params"].(map[string]any)
		writeJSON(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"sessionId": "sess-xyz"},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	sid, err := conn.AttachToTarget(ctx, "target-1")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if sid != "sess-xyz" {
		t.Errorf("got sessionId %q, want sess-xyz", sid)
	}
	if seenParams["targetId"] != "target-1" {
		t.Errorf("server saw targetId %v, want target-1", seenParams["targetId"])
	}
	if seenParams["flatten"] != true {
		t.Errorf("server saw flatten %v, want true", seenParams["flatten"])
	}
}

func TestAttachToTargetSendError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{
			"id":    f["id"],
			"error": map[string]any{"code": -32000, "message": "no such target"},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.AttachToTarget(ctx, "missing"); err == nil {
		t.Fatal("expected error from AttachToTarget when Send fails")
	}
}

func TestAttachToTargetDecodeError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		// sessionId as a number -> unmarshal into string fails.
		writeJSON(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"sessionId": 12345},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, err = conn.AttachToTarget(ctx, "t")
	if err == nil || !strings.Contains(err.Error(), "decode attachToTarget") {
		t.Fatalf("expected decode attachToTarget error, got %v", err)
	}
}

// --- CreateTarget --------------------------------------------------------

func TestCreateTargetReturnsTargetID(t *testing.T) {
	var seenURL any
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		if p, ok := f["params"].(map[string]any); ok {
			seenURL = p["url"]
		}
		writeJSON(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"targetId": "new-tgt"},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	tid, err := conn.CreateTarget(ctx, "https://example.com/page")
	if err != nil {
		t.Fatalf("createTarget: %v", err)
	}
	if tid != "new-tgt" {
		t.Errorf("got targetId %q, want new-tgt", tid)
	}
	if seenURL != "https://example.com/page" {
		t.Errorf("server saw url %v", seenURL)
	}
}

func TestCreateTargetSendError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{
			"id":    f["id"],
			"error": map[string]any{"code": -32000, "message": "cannot create"},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.CreateTarget(ctx, "https://example.com"); err == nil {
		t.Fatal("expected error from CreateTarget when Send fails")
	}
}

func TestCreateTargetDecodeError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"targetId": []int{1, 2}},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, err = conn.CreateTarget(ctx, "https://example.com")
	if err == nil || !strings.Contains(err.Error(), "decode createTarget") {
		t.Fatalf("expected decode createTarget error, got %v", err)
	}
}

// --- SubscribeMethods ----------------------------------------------------

func TestSubscribeMethodsReceivesAllMethods(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		// On any command, emit two different events then reply.
		writeJSON(t, ws, map[string]any{
			"method": "Network.requestWillBeSent",
			"params": map[string]any{"requestId": "r1"},
		})
		writeJSON(t, ws, map[string]any{
			"method": "Network.responseReceived",
			"params": map[string]any{"requestId": "r1"},
		})
		writeJSON(t, ws, map[string]any{"id": f["id"], "result": map[string]any{}})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ch, cancelSub := conn.SubscribeMethods([]string{
		"Network.requestWillBeSent",
		"Network.responseReceived",
	}, 8)
	defer cancelSub()

	if _, err := conn.Send(ctx, "", "Trigger.events", nil); err != nil {
		t.Fatalf("send: %v", err)
	}

	got := make([]string, 0, 2)
	deadline := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("channel closed early")
			}
			got = append(got, ev.Method)
		case <-deadline:
			t.Fatalf("timed out; got %v", got)
		}
	}
	if got[0] != "Network.requestWillBeSent" || got[1] != "Network.responseReceived" {
		t.Errorf("ordering wrong, got %v", got)
	}
}

func TestSubscribeMethodsCancelRemovesFromEveryMethod(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{"id": f["id"], "result": map[string]any{}})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	methods := []string{"A.one", "B.two", "C.three"}
	ch, cancelSub := conn.SubscribeMethods(methods, 4)

	conn.mu.Lock()
	for _, m := range methods {
		if len(conn.subs[m]) != 1 {
			conn.mu.Unlock()
			t.Fatalf("subs[%s] = %d, want 1 before cancel", m, len(conn.subs[m]))
		}
	}
	conn.mu.Unlock()

	cancelSub()

	conn.mu.Lock()
	for _, m := range methods {
		if len(conn.subs[m]) != 0 {
			conn.mu.Unlock()
			t.Fatalf("subs[%s] = %d, want 0 after cancel", m, len(conn.subs[m]))
		}
	}
	conn.mu.Unlock()

	// Channel must be closed exactly once by cancel.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected closed channel after cancel")
		}
	case <-time.After(time.Second):
		t.Error("channel not closed after cancel")
	}

	// Second cancel is a no-op (covers the closed==true guard via map lookups).
	cancelSub()
}

func TestSubscribeMethodsOnClosedConnection(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
	<-conn.Done()

	ch, cancelSub := conn.SubscribeMethods([]string{"X.y"}, 1)
	// Channel returned for a closed connection is already closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected pre-closed channel on closed connection")
		}
	case <-time.After(time.Second):
		t.Error("channel not closed for closed connection")
	}
	// cancel on closed connection is a no-op.
	cancelSub()
}

func TestSubscribeMethodsCancelAfterClose(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	_, cancelSub := conn.SubscribeMethods([]string{"M.one", "M.two"}, 2)
	_ = conn.Close()
	<-conn.Done()
	// cancel after the connection closed hits the closed guard inside cancel.
	cancelSub()
}

// --- closeWithErr subscriber cleanup -------------------------------------

func TestCloseClosesSubscriberChannels(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	single, _ := conn.Subscribe("Single.evt", 1)
	// SubscribeMethods shares one channel across multiple methods; Close must
	// close that shared channel exactly once (the closed-map dedupe path).
	multi, _ := conn.SubscribeMethods([]string{"Multi.a", "Multi.b"}, 1)

	_ = conn.Close()
	<-conn.Done()

	for name, ch := range map[string]<-chan Event{"single": single, "multi": multi} {
		select {
		case _, ok := <-ch:
			if ok {
				t.Errorf("%s channel not closed by Close", name)
			}
		case <-time.After(time.Second):
			t.Errorf("%s channel not closed by Close (timeout)", name)
		}
	}
}

// --- Dial error path -----------------------------------------------------

func TestDialBadURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := Dial(ctx, "ws://127.0.0.1:0/nonexistent")
	if err == nil {
		t.Fatal("expected dial error for unreachable endpoint")
	}
	if !strings.Contains(err.Error(), "cdp dial") {
		t.Errorf("error %q missing 'cdp dial' prefix", err.Error())
	}
}

// --- Send marshal / write error paths ------------------------------------

func TestSendMarshalError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{"id": f["id"], "result": map[string]any{}})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// A channel value cannot be JSON-marshaled; this exercises the marshal
	// error branch (and its cleanup of the pending entry).
	_, err = conn.Send(ctx, "", "Bad.params", make(chan int))
	if err == nil || !strings.Contains(err.Error(), "cdp marshal") {
		t.Fatalf("expected marshal error, got %v", err)
	}

	conn.mu.Lock()
	npending := len(conn.pending)
	conn.mu.Unlock()
	if npending != 0 {
		t.Errorf("pending not cleaned up after marshal error: %d", npending)
	}
}

func TestSendWriteErrorAfterPeerClose(t *testing.T) {
	// Server closes the socket right after the first inbound read, so a
	// subsequent write from the client fails. We need the read pump to still
	// be alive when we attempt the failing write, so we don't rely on it.
	closed := make(chan struct{})
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		// Drop the connection without replying.
		_ = ws.Close()
		select {
		case <-closed:
		default:
			close(closed)
		}
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// First send triggers the server to close. It may fail with a write error,
	// a closed error, or a context-bounded read failure depending on timing;
	// any error is acceptable. The point is exercising Send under a broken
	// socket without a panic.
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer sendCancel()
	_, _ = conn.Send(sendCtx, "", "Will.break", nil)
	<-closed
}

// --- ResolveBrowserWSURL extra paths -------------------------------------

func TestResolveBrowserWSURLParseError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// A control character makes url.Parse fail.
	_, err := ResolveBrowserWSURL(ctx, "http://\x7f/")
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestResolveBrowserWSURLRequestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := srv.URL
	srv.Close() // nothing is listening now

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := ResolveBrowserWSURL(ctx, addr)
	if err == nil || !strings.Contains(err.Error(), "probe") {
		t.Fatalf("expected probe error, got %v", err)
	}
}

func TestResolveBrowserWSURLDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := ResolveBrowserWSURL(ctx, srv.URL)
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestResolveBrowserWSURLMissingURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Browser":"Chrome/127"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := ResolveBrowserWSURL(ctx, srv.URL)
	if err == nil || !strings.Contains(err.Error(), "missing webSocketDebuggerUrl") {
		t.Fatalf("expected missing URL error, got %v", err)
	}
}

func TestResolveBrowserWSURLTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"ws://x/y"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Trailing slash on the browser URL exercises the TrimSuffix branch.
	got, err := ResolveBrowserWSURL(ctx, srv.URL+"/")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "ws://x/y" {
		t.Errorf("got %q, want ws://x/y", got)
	}
}

// --- Evaluate decode error -----------------------------------------------

func TestEvaluateDecodeError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		// result.result as a string can't unmarshal into *RemoteObject.
		writeJSON(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"result": "not-an-object"},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, err = conn.Evaluate(ctx, "sess", EvaluateParams{Expression: "1"})
	if err == nil || !strings.Contains(err.Error(), "decode evaluate") {
		t.Fatalf("expected decode evaluate error, got %v", err)
	}
}

func TestEvaluateSendError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{
			"id":    f["id"],
			"error": map[string]any{"code": -32000, "message": "eval failed"},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Evaluate(ctx, "sess", EvaluateParams{Expression: "boom"}); err == nil {
		t.Fatal("expected Send error to propagate from Evaluate")
	}
}

// --- ExceptionDetails.String branches ------------------------------------

func TestExceptionDetailsStringNil(t *testing.T) {
	var e *ExceptionDetails
	if e.String() != "" {
		t.Errorf("nil String() = %q, want empty", e.String())
	}
}

func TestExceptionDetailsStringTextFallback(t *testing.T) {
	// No exception object -> falls back to Text.
	e := &ExceptionDetails{Text: "Uncaught SyntaxError"}
	if e.String() != "Uncaught SyntaxError" {
		t.Errorf("String() = %q, want text fallback", e.String())
	}

	// Exception present but no description -> still uses Text.
	e2 := &ExceptionDetails{Text: "from-text", Exception: &RemoteObject{Type: "object"}}
	if e2.String() != "from-text" {
		t.Errorf("String() = %q, want from-text", e2.String())
	}

	// Exception with description -> uses description.
	e3 := &ExceptionDetails{Text: "ignored", Exception: &RemoteObject{Description: "TypeError: x"}}
	if e3.String() != "TypeError: x" {
		t.Errorf("String() = %q, want description", e3.String())
	}
}

// --- PageNavigate decode error -------------------------------------------

func TestPageNavigateDecodeError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"frameId": []int{1}},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, err = conn.PageNavigate(ctx, "sess", "https://x")
	if err == nil || !strings.Contains(err.Error(), "decode page.navigate") {
		t.Fatalf("expected decode page.navigate error, got %v", err)
	}
}

func TestPageNavigateSendError(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{
			"id":    f["id"],
			"error": map[string]any{"code": -32000, "message": "nav failed"},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := conn.PageNavigate(ctx, "sess", "https://x"); err == nil {
		t.Fatal("expected Send error to propagate from PageNavigate")
	}
}

// --- isInternalURL edge cases --------------------------------------------

func TestIsInternalURLVariants(t *testing.T) {
	cases := map[string]bool{
		"devtools://devtools/x":       true,
		"chrome://newtab/":            true,
		"edge://settings":             true,
		"https://app.example/index":   false,
		"":                            false,
		"about:blank":                 false,
		"file:///c:/x.html":           false,
		"chrome-extension://abc/page": false, // not chrome:// prefix exactly
	}
	for u, want := range cases {
		if got := isInternalURL(u); got != want {
			t.Errorf("isInternalURL(%q) = %v, want %v", u, got, want)
		}
	}
}

// --- Send returns RemoteError via errors.As over wire --------------------

func TestSendRemoteErrorWithData(t *testing.T) {
	wsURL, stop := fakeCDP(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		writeJSON(t, ws, map[string]any{
			"id": f["id"],
			"error": map[string]any{
				"code":    -32602,
				"message": "Invalid params",
				"data":    "missing field",
			},
		})
	})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, err = conn.Send(ctx, "sess", "Bad.cmd", map[string]any{"x": 1})
	var rerr *RemoteError
	if !errors.As(err, &rerr) {
		t.Fatalf("expected *RemoteError, got %T: %v", err, err)
	}
	if rerr.Data != "missing field" {
		t.Errorf("Data = %q, want 'missing field'", rerr.Data)
	}
	if !strings.Contains(rerr.Error(), "missing field") {
		t.Errorf("Error() = %q, want data included", rerr.Error())
	}
}
