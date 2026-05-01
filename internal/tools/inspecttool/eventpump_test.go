package inspecttool

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	cdpproto "github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/session"
)

// drainBuf polls a buffer until it has at least n records or fails the test.
// The pump goroutine is asynchronous, so direct .Drain may race the append.
func drainBuf(t *testing.T, buf *session.EventBuf, n int) []session.EventRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		res := buf.Drain(session.DrainOpts{})
		if len(res.Records) >= n {
			return res.Records
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d records, have %d", n, len(res.Records))
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestPumpConsole_FiltersBySessionAndTagsKind(t *testing.T) {
	sess := session.NewManager(session.Config{}).Get("default")
	ch := make(chan cdpproto.Event, 16)

	target := sess.EventBuf(session.ConsoleBufKind, "sid-A", 100)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pumpConsole(target, "sid-A", ch)
	}()

	// Wrong session — must be ignored.
	ch <- cdpproto.Event{
		SessionID: "sid-B",
		Method:    "Runtime.consoleAPICalled",
		Params:    json.RawMessage(`{"type":"log","args":[{"type":"string","value":"ignored"}]}`),
	}
	// Right session, type=warn → kind=console.warn
	ch <- cdpproto.Event{
		SessionID: "sid-A",
		Method:    "Runtime.consoleAPICalled",
		Params:    json.RawMessage(`{"type":"warn","args":[{"type":"string","value":"hi"}]}`),
	}
	ch <- cdpproto.Event{
		SessionID: "sid-A",
		Method:    "Runtime.exceptionThrown",
		Params:    json.RawMessage(`{"exceptionDetails":{"text":"boom"}}`),
	}
	ch <- cdpproto.Event{
		SessionID: "sid-A",
		Method:    "Log.entryAdded",
		Params:    json.RawMessage(`{"entry":{"source":"deprecation","level":"warning","text":"old api"}}`),
	}

	got := drainBuf(t, target, 3)

	close(ch)
	wg.Wait()

	// Cross-channel ordering isn't deterministic (select picks at random),
	// so assert on the kind set rather than positions.
	kinds := map[string]bool{}
	for _, r := range got {
		kinds[r.Kind] = true
	}
	for _, want := range []string{"console.warn", "exception", "log.entry"} {
		if !kinds[want] {
			t.Errorf("missing record kind=%q; got kinds=%v", want, kinds)
		}
	}
}

func TestPumpNetwork_CorrelatesRequestLifecycle(t *testing.T) {
	sess := session.NewManager(session.Config{}).Get("default")
	target := sess.EventBuf(session.NetworkBufKind, "sid-A", 100)
	ch := make(chan cdpproto.Event, 16)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pumpNetwork(target, "sid-A", ch)
	}()

	ch <- cdpproto.Event{
		SessionID: "sid-A",
		Method:    "Network.requestWillBeSent",
		Params: json.RawMessage(`{
			"requestId":"req-1",
			"request":{"url":"https://contoso/api","method":"GET","headers":{"x-trace":"abc"}},
			"type":"XHR",
			"timestamp": 100.0
		}`),
	}
	ch <- cdpproto.Event{
		SessionID: "sid-A",
		Method:    "Network.responseReceived",
		Params: json.RawMessage(`{
			"requestId":"req-1",
			"response":{"status":200,"statusText":"OK","mimeType":"application/json","headers":{}}
		}`),
	}
	ch <- cdpproto.Event{
		SessionID: "sid-A",
		Method:    "Network.loadingFinished",
		Params:    json.RawMessage(`{"requestId":"req-1","timestamp":100.5,"encodedDataLength":1234}`),
	}

	// A failed request without a matching willSend — should still be
	// emitted (the orphan-rescue path in finalizeFailed).
	ch <- cdpproto.Event{
		SessionID: "sid-A",
		Method:    "Network.loadingFailed",
		Params:    json.RawMessage(`{"requestId":"req-orphan","errorText":"net::ERR","canceled":false}`),
	}

	got := drainBuf(t, target, 2)

	close(ch)
	wg.Wait()

	// Cross-channel ordering is non-deterministic; key by requestId.
	byID := map[string]struct {
		kind string
		rec  networkRecord
	}{}
	for _, r := range got {
		var rec networkRecord
		if err := json.Unmarshal(r.Data, &rec); err != nil {
			t.Fatalf("decode: %v", err)
		}
		byID[rec.RequestID] = struct {
			kind string
			rec  networkRecord
		}{kind: r.Kind, rec: rec}
	}

	one, ok := byID["req-1"]
	if !ok {
		t.Fatalf("missing record for req-1; got=%v", byID)
	}
	if one.kind != "network.complete" {
		t.Errorf("req-1 kind=%q, want network.complete", one.kind)
	}
	if one.rec.URL != "https://contoso/api" || one.rec.Method != "GET" {
		t.Errorf("req-1 metadata lost: %+v", one.rec)
	}
	if one.rec.Status != 200 || one.rec.MimeType != "application/json" {
		t.Errorf("req-1 response fields not merged: %+v", one.rec)
	}
	if one.rec.Size != 1234 {
		t.Errorf("req-1 size lost: %d", one.rec.Size)
	}
	if one.rec.DurationMs != 500 {
		t.Errorf("req-1 duration=%d, want 500", one.rec.DurationMs)
	}
	if one.rec.Failed {
		t.Errorf("req-1 must not be marked failed: %+v", one.rec)
	}

	orphan, ok := byID["req-orphan"]
	if !ok {
		t.Fatalf("missing record for req-orphan; got=%v", byID)
	}
	if orphan.kind != "network.failed" {
		t.Errorf("orphan kind=%q, want network.failed", orphan.kind)
	}
	if !orphan.rec.Failed || orphan.rec.ErrorText != "net::ERR" {
		t.Errorf("orphan failure not surfaced: %+v", orphan.rec)
	}
}

func TestNormalizeConsoleAPI(t *testing.T) {
	cases := []struct {
		name     string
		params   string
		wantText string
		wantSrc  string
	}{
		{
			name:     "string arg",
			params:   `{"type":"log","args":[{"type":"string","value":"hello world"}]}`,
			wantText: "hello world",
		},
		{
			name:     "number and bool args",
			params:   `{"type":"log","args":[{"type":"number","value":42},{"type":"boolean","value":true}]}`,
			wantText: "42 true",
		},
		{
			name:     "null and undefined",
			params:   `{"type":"log","args":[{"type":"object","subtype":"null"},{"type":"undefined"}]}`,
			wantText: "null undefined",
		},
		{
			name:     "object uses description",
			params:   `{"type":"log","args":[{"type":"object","description":"Array(3)","objectId":"1"}]}`,
			wantText: "Array(3)",
		},
		{
			name:     "stack trace becomes src",
			params:   `{"type":"log","args":[{"type":"string","value":"hi"}],"stackTrace":{"callFrames":[{"url":"https://localhost:3000/taskpane.js","lineNumber":9}]}}`,
			wantText: "hi",
			wantSrc:  "taskpane.js:10",
		},
		{
			name:     "multiple string args joined",
			params:   `{"type":"log","args":[{"type":"string","value":"a"},{"type":"string","value":"b"},{"type":"string","value":"c"}]}`,
			wantText: "a b c",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := normalizeConsoleAPI(json.RawMessage(tc.params))
			var entry consoleEntry
			if err := json.Unmarshal(out, &entry); err != nil {
				t.Fatalf("unmarshal: %v (raw=%s)", err, out)
			}
			if entry.Text != tc.wantText {
				t.Errorf("text=%q, want %q", entry.Text, tc.wantText)
			}
			if entry.Src != tc.wantSrc {
				t.Errorf("src=%q, want %q", entry.Src, tc.wantSrc)
			}
		})
	}
}

func TestNormalizeException(t *testing.T) {
	raw := `{"exceptionDetails":{"text":"Uncaught Error","lineNumber":4,"url":"https://localhost:3000/app.js","exception":{"type":"object","subtype":"error","description":"Error: boom\n    at app.js:5:1"}}}`
	out := normalizeException(json.RawMessage(raw))
	var entry consoleEntry
	if err := json.Unmarshal(out, &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.Text != "Error: boom\n    at app.js:5:1" {
		t.Errorf("text=%q", entry.Text)
	}
	if entry.Src != "app.js:5" {
		t.Errorf("src=%q, want app.js:5", entry.Src)
	}
}

func TestNormalizeLogEntry(t *testing.T) {
	raw := `{"entry":{"source":"javascript","level":"warning","text":"old api deprecated","url":"https://localhost:3000/lib.js","lineNumber":42}}`
	out := normalizeLogEntry(json.RawMessage(raw))
	var entry consoleEntry
	if err := json.Unmarshal(out, &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.Text != "old api deprecated" {
		t.Errorf("text=%q", entry.Text)
	}
	if entry.Src != "lib.js:42" {
		t.Errorf("src=%q, want lib.js:42", entry.Src)
	}
}

func TestConsoleKindFromParams(t *testing.T) {
	cases := map[string]string{
		`{"type":"log"}`:   "console.log",
		`{"type":"error"}`: "console.error",
		`{"type":""}`:      "console",
		`not json`:         "console",
	}
	for in, want := range cases {
		if got := consoleKindFromParams(json.RawMessage(in)); got != want {
			t.Errorf("consoleKindFromParams(%q) = %q, want %q", in, got, want)
		}
	}
}
