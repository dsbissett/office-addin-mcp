package inspecttool

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	cdpproto "github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
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

// --- malformed-input fallback paths for the normalizers ---

func TestNormalizeConsoleAPI_BadJSONReturnsRaw(t *testing.T) {
	raw := json.RawMessage(`{not json`)
	out := normalizeConsoleAPI(raw)
	if string(out) != string(raw) {
		t.Errorf("bad json should pass through unchanged, got %s", out)
	}
}

func TestNormalizeException_BadJSONReturnsRaw(t *testing.T) {
	raw := json.RawMessage(`{not json`)
	out := normalizeException(raw)
	if string(out) != string(raw) {
		t.Errorf("bad json should pass through unchanged, got %s", out)
	}
}

func TestNormalizeException_FallsBackToText(t *testing.T) {
	// No exception.description and a stackTrace → src from frame, text from
	// exceptionDetails.text.
	raw := `{"exceptionDetails":{"text":"plain text error","stackTrace":{"callFrames":[{"url":"https://x/main.js","lineNumber":3}]}}}`
	out := normalizeException(json.RawMessage(raw))
	var entry consoleEntry
	if err := json.Unmarshal(out, &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.Text != "plain text error" {
		t.Errorf("text=%q", entry.Text)
	}
	if entry.Src != "main.js:4" {
		t.Errorf("src=%q, want main.js:4", entry.Src)
	}
}

func TestNormalizeLogEntry_BadJSONReturnsRaw(t *testing.T) {
	raw := json.RawMessage(`{not json`)
	out := normalizeLogEntry(raw)
	if string(out) != string(raw) {
		t.Errorf("bad json should pass through unchanged, got %s", out)
	}
}

func TestNormalizeLogEntry_NoURLLeavesSrcEmpty(t *testing.T) {
	out := normalizeLogEntry(json.RawMessage(`{"entry":{"text":"no url here"}}`))
	var entry consoleEntry
	if err := json.Unmarshal(out, &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.Src != "" {
		t.Errorf("src=%q, want empty", entry.Src)
	}
}

// --- argText edge cases ---

func TestArgText(t *testing.T) {
	cases := []struct {
		name string
		arg  cdpArg
		want string
	}{
		{"string", cdpArg{Type: "string", Value: json.RawMessage(`"hi"`)}, "hi"},
		{"string bad value falls through to raw", cdpArg{Type: "string", Value: json.RawMessage(`123`)}, "123"},
		{"undefined", cdpArg{Type: "undefined"}, "undefined"},
		{"object null", cdpArg{Type: "object", Subtype: "null"}, "null"},
		{"object description", cdpArg{Type: "object", Description: "Array(2)"}, "Array(2)"},
		{"object no description falls to raw value", cdpArg{Type: "object", Value: json.RawMessage(`{"a":1}`)}, `{"a":1}`},
		{"function description", cdpArg{Type: "function", Description: "function f()"}, "function f()"},
		{"unserializable", cdpArg{Type: "number", UnserializableValue: "Infinity"}, "Infinity"},
		{"number raw value", cdpArg{Type: "number", Value: json.RawMessage(`42`)}, "42"},
		{"empty description fallback", cdpArg{Type: "symbol", Description: "Symbol(x)"}, "Symbol(x)"},
		{"nothing", cdpArg{Type: "weird"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := argText(tc.arg); got != tc.want {
				t.Errorf("argText=%q, want %q", got, tc.want)
			}
		})
	}
}

// --- shortFile ---

func TestShortFile(t *testing.T) {
	cases := map[string]string{
		"https://localhost:3000/taskpane.js":  "taskpane.js",
		"https://localhost:3000/a/b/c.js?v=2": "c.js",
		"file.js":                             "file.js",
		"https://host/path?onlyquery=1":       "path",
		"":                                    "",
		"https://localhost/dir/":              "",
	}
	for in, want := range cases {
		if got := shortFile(in); got != want {
			t.Errorf("shortFile(%q)=%q, want %q", in, got, want)
		}
	}
}

// --- frameSrc ---

func TestFrameSrc(t *testing.T) {
	if got := frameSrc(nil); got != "" {
		t.Errorf("nil stack=%q, want empty", got)
	}
	empty := &cdpStackTrace{}
	if got := frameSrc(empty); got != "" {
		t.Errorf("no frames=%q, want empty", got)
	}
	st := &cdpStackTrace{}
	st.CallFrames = append(st.CallFrames, struct {
		URL        string `json:"url"`
		LineNumber int    `json:"lineNumber"`
	}{URL: "", LineNumber: 0})
	st.CallFrames = append(st.CallFrames, struct {
		URL        string `json:"url"`
		LineNumber int    `json:"lineNumber"`
	}{URL: "https://x/app.js", LineNumber: 5})
	// First frame has empty URL → skipped; second gives src (1-based).
	if got := frameSrc(st); got != "app.js:6" {
		t.Errorf("frameSrc=%q, want app.js:6", got)
	}
}

// --- marshalEntry ---

func TestMarshalEntry(t *testing.T) {
	out := marshalEntry(consoleEntry{Text: "t", Src: "s.js:1"}, json.RawMessage(`"fallback"`))
	var e consoleEntry
	if err := json.Unmarshal(out, &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Text != "t" || e.Src != "s.js:1" {
		t.Errorf("entry=%+v", e)
	}
}

// --- buildNetworkRecord ---

func TestBuildNetworkRecord_DurationAndFields(t *testing.T) {
	cur := &pendingRequest{
		url: "u", method: "POST", resType: "Fetch", status: 201, statusTxt: "Created",
		mimeType: "application/json", size: 99, t0: 10, t1: 10.5,
	}
	rec := buildNetworkRecord("rid", cur, false, "", false)
	if rec.RequestID != "rid" || rec.URL != "u" || rec.Method != "POST" {
		t.Errorf("rec basics wrong: %+v", rec)
	}
	if rec.DurationMs != 500 {
		t.Errorf("duration=%d, want 500", rec.DurationMs)
	}
	if rec.Status != 201 || rec.Size != 99 {
		t.Errorf("rec fields wrong: %+v", rec)
	}
}

func TestBuildNetworkRecord_NoDurationWhenNoStart(t *testing.T) {
	// t0 == 0 → no duration computed even with t1 set (orphan-rescue path).
	cur := &pendingRequest{t0: 0, t1: 5}
	rec := buildNetworkRecord("rid", cur, true, "net::ERR", true)
	if rec.DurationMs != 0 {
		t.Errorf("duration=%d, want 0 (no start time)", rec.DurationMs)
	}
	if !rec.Failed || rec.ErrorText != "net::ERR" || !rec.Canceled {
		t.Errorf("failure fields wrong: %+v", rec)
	}
}

// --- pendingFifo ---

func TestPendingFifo_PutTakePeek(t *testing.T) {
	f := newPendingFifo()
	f.put("a", &pendingRequest{url: "ua"})
	f.put("b", &pendingRequest{url: "ub"})
	if p := f.peek("a"); p == nil || p.url != "ua" {
		t.Errorf("peek a wrong: %+v", p)
	}
	// peek does not remove.
	if p := f.peek("a"); p == nil {
		t.Errorf("peek must not remove")
	}
	if p := f.take("b"); p == nil || p.url != "ub" {
		t.Errorf("take b wrong: %+v", p)
	}
	// take again → nil (already removed).
	if p := f.take("b"); p != nil {
		t.Errorf("double-take should return nil")
	}
	// take missing → nil.
	if p := f.take("zzz"); p != nil {
		t.Errorf("take missing should return nil")
	}
}

func TestPendingFifo_PutSameIDDoesNotDuplicateOrder(t *testing.T) {
	f := newPendingFifo()
	f.put("a", &pendingRequest{url: "v1"})
	f.put("a", &pendingRequest{url: "v2"})
	if len(f.order) != 1 {
		t.Errorf("order should not duplicate same id, len=%d", len(f.order))
	}
	if p := f.peek("a"); p == nil || p.url != "v2" {
		t.Errorf("put should overwrite value: %+v", p)
	}
}

func TestPendingFifo_Eviction(t *testing.T) {
	f := newPendingFifo()
	// Fill beyond the cap; the oldest must be evicted.
	total := pendingPumpCap + 5
	for i := 0; i < total; i++ {
		f.put(itoa(i), &pendingRequest{url: itoa(i)})
	}
	if len(f.data) > pendingPumpCap {
		t.Errorf("data size=%d exceeds cap %d", len(f.data), pendingPumpCap)
	}
	// The very first insertions should have been evicted.
	if f.peek("0") != nil {
		t.Errorf("oldest entry should have been evicted")
	}
	// A recent entry survives.
	if f.peek(itoa(total-1)) == nil {
		t.Errorf("newest entry should survive")
	}
}

func itoa(i int) string {
	// small helper to avoid importing strconv for one use
	if i == 0 {
		return "0"
	}
	var b []byte
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// --- handleWillSend / handleRespRecv bad-input guards ---

func TestHandleWillSend_BadInputIgnored(t *testing.T) {
	f := newPendingFifo()
	handleWillSend(f, json.RawMessage(`{not json`))
	handleWillSend(f, json.RawMessage(`{"requestId":""}`)) // empty id
	if len(f.data) != 0 {
		t.Errorf("bad will-send frames should be ignored, have %d", len(f.data))
	}
}

func TestHandleRespRecv_MaterializesWhenNoPending(t *testing.T) {
	f := newPendingFifo()
	// No prior willSend; respRecv must materialize an entry.
	handleRespRecv(f, json.RawMessage(`{"requestId":"r1","response":{"status":204,"mimeType":"text/plain"}}`))
	cur := f.peek("r1")
	if cur == nil || cur.status != 204 || !cur.hasResp {
		t.Errorf("respRecv should materialize entry: %+v", cur)
	}
	// Bad input is ignored.
	handleRespRecv(f, json.RawMessage(`{bad`))
	handleRespRecv(f, json.RawMessage(`{"requestId":""}`))
}

// --- finalizeFinished / finalizeFailed bad-input guards ---

func TestFinalize_BadInputDoesNotAppend(t *testing.T) {
	buf := newSession().EventBuf(session.NetworkBufKind, "sid", 100)
	f := newPendingFifo()
	finalizeFinished(buf, f, json.RawMessage(`{bad`))
	finalizeFinished(buf, f, json.RawMessage(`{"requestId":""}`))
	finalizeFailed(buf, f, json.RawMessage(`{bad`))
	finalizeFailed(buf, f, json.RawMessage(`{"requestId":""}`))
	if got := buf.Drain(session.DrainOpts{}); len(got.Records) != 0 {
		t.Errorf("bad frames should not append, have %d", len(got.Records))
	}
}

// --- ensureConsolePump / ensureNetworkPump via the in-process server ---

func TestEnsureConsolePump_IdempotentAndEnableError(t *testing.T) {
	sess := newSession()
	conn := cdptestServer(t, func(string, json.RawMessage) (any, *cdpproto.RemoteError) {
		return map[string]any{}, nil
	})
	env := pumpEnv(sess, nil)
	// First call spawns the pump and enables Runtime + Log.
	if err := ensureConsolePump(context.Background(), env, conn, "sid-X", 100); err != nil {
		t.Fatalf("first ensureConsolePump: %v", err)
	}
	// Second call is a no-op (already pumping) and must not error.
	if err := ensureConsolePump(context.Background(), env, conn, "sid-X", 100); err != nil {
		t.Fatalf("second ensureConsolePump: %v", err)
	}
}

func TestEnsureConsolePump_EnableError(t *testing.T) {
	sess := newSession()
	conn := cdptestServer(t, func(string, json.RawMessage) (any, *cdpproto.RemoteError) {
		return map[string]any{}, nil
	})
	env := pumpEnv(sess, errors.New("enable failed"))
	err := ensureConsolePump(context.Background(), env, conn, "sid-E", 100)
	if err == nil {
		t.Fatal("expected an enable error to propagate")
	}
}

func TestEnsureNetworkPump_IdempotentAndEnableError(t *testing.T) {
	sess := newSession()
	conn := cdptestServer(t, func(string, json.RawMessage) (any, *cdpproto.RemoteError) {
		return map[string]any{}, nil
	})
	env := pumpEnv(sess, nil)
	if err := ensureNetworkPump(context.Background(), env, conn, "sid-N", 100); err != nil {
		t.Fatalf("first ensureNetworkPump: %v", err)
	}
	if err := ensureNetworkPump(context.Background(), env, conn, "sid-N", 100); err != nil {
		t.Fatalf("second ensureNetworkPump: %v", err)
	}

	envErr := pumpEnv(sess, errors.New("enable failed"))
	if err := ensureNetworkPump(context.Background(), envErr, conn, "sid-NE", 100); err == nil {
		t.Fatal("expected an enable error to propagate")
	}
}

// pumpEnv builds a minimal RunEnv backing EventBuf / MarkEventPumping with sess
// and an EnsureEnabled that returns enableErr.
func pumpEnv(sess *session.Session, enableErr error) *tools.RunEnv {
	return &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		EnsureEnabled: func(context.Context, string, string) error {
			return enableErr
		},
		EventBuf: func(kind session.EventBufKind, cdpSessionID string, max int) *session.EventBuf {
			return sess.EventBuf(kind, cdpSessionID, max)
		},
		MarkEventPumping: func(kind session.EventBufKind, cdpSessionID string, max int) bool {
			return sess.MarkEventPumping(kind, cdpSessionID, max)
		},
	}
}
