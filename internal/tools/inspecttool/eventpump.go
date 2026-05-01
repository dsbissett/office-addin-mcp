package inspecttool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	cdpproto "github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// subscribeBuffer is the chan size handed to cdp.Connection.Subscribe.
// connection.go drops events on slow subscribers (non-blocking send), so
// this needs to be roomy enough that an event burst doesn't get dropped
// while the pump goroutine is still mid-append.
const subscribeBuffer = 256

// ensureConsolePump arranges for Runtime.consoleAPICalled,
// Runtime.exceptionThrown and Log.entryAdded events on the given
// cdpSessionID to be appended to the per-target console ring buffer.
// Idempotent: if a pump is already running for (session, cdpSessionID),
// the call only reapplies maxBuffer.
func ensureConsolePump(ctx context.Context, env *tools.RunEnv, conn *cdpproto.Connection, cdpSID string, maxBuffer int) error {
	buf := env.EventBuf(session.ConsoleBufKind, cdpSID, maxBuffer)
	if !env.MarkEventPumping(session.ConsoleBufKind, cdpSID, maxBuffer) {
		return nil
	}
	// Subscribe before enabling: Chrome replays buffered console messages as
	// Runtime.consoleAPICalled events immediately after Runtime.enable is
	// acknowledged. If we subscribed after enabling, those replay events would
	// be dropped by the non-blocking send in the read loop.
	ch, _ := conn.SubscribeMethods([]string{
		"Runtime.consoleAPICalled",
		"Runtime.exceptionThrown",
		"Log.entryAdded",
	}, subscribeBuffer)
	go pumpConsole(buf, cdpSID, ch)
	if err := env.EnsureEnabled(ctx, cdpSID, "Runtime"); err != nil {
		return err
	}
	if err := env.EnsureEnabled(ctx, cdpSID, "Log"); err != nil {
		return err
	}
	return nil
}

func pumpConsole(buf *session.EventBuf, cdpSID string, ch <-chan cdpproto.Event) {
	for ev := range ch {
		if ev.SessionID != cdpSID {
			continue
		}
		switch ev.Method {
		case "Runtime.consoleAPICalled":
			buf.Append(consoleKindFromParams(ev.Params), normalizeConsoleAPI(ev.Params))
		case "Runtime.exceptionThrown":
			buf.Append("exception", normalizeException(ev.Params))
		case "Log.entryAdded":
			buf.Append("log.entry", normalizeLogEntry(ev.Params))
		}
	}
}

// consoleKindFromParams pulls the console method (log/warn/error/...) from a
// Runtime.consoleAPICalled event so callers can filter by level without
// re-parsing the raw payload. Falls back to "console" on a malformed frame.
func consoleKindFromParams(raw json.RawMessage) string {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil || head.Type == "" {
		return "console"
	}
	return "console." + head.Type
}

// consoleEntry is the compact form stored in the ring buffer for every console
// event. It replaces the raw CDP payload (RemoteObject trees, objectIds, stack
// frames) with a single human-readable string and an optional short source
// location.
type consoleEntry struct {
	Text string `json:"text"`
	Src  string `json:"src,omitempty"` // "file.js:N" (1-based)
}

// CDP parsing types — unexported, used only for normalization.
type cdpArg struct {
	Type                string          `json:"type"`
	Subtype             string          `json:"subtype,omitempty"`
	Value               json.RawMessage `json:"value,omitempty"`
	UnserializableValue string          `json:"unserializableValue,omitempty"`
	Description         string          `json:"description,omitempty"`
}

type cdpStackTrace struct {
	CallFrames []struct {
		URL        string `json:"url"`
		LineNumber int    `json:"lineNumber"` // 0-based per CDP spec
	} `json:"callFrames"`
}

// normalizeConsoleAPI converts a Runtime.consoleAPICalled payload into a
// compact consoleEntry by rendering args as human-readable text.
func normalizeConsoleAPI(raw json.RawMessage) json.RawMessage {
	var ev struct {
		Args       []cdpArg       `json:"args"`
		StackTrace *cdpStackTrace `json:"stackTrace,omitempty"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return raw
	}
	parts := make([]string, 0, len(ev.Args))
	for _, a := range ev.Args {
		parts = append(parts, argText(a))
	}
	return marshalEntry(consoleEntry{
		Text: strings.Join(parts, " "),
		Src:  frameSrc(ev.StackTrace),
	}, raw)
}

// normalizeException converts a Runtime.exceptionThrown payload into a compact
// consoleEntry using the exception description (includes message + stack as
// text) when available, falling back to exceptionDetails.text.
func normalizeException(raw json.RawMessage) json.RawMessage {
	var ev struct {
		ExceptionDetails struct {
			Text       string         `json:"text"`
			LineNumber int            `json:"lineNumber"` // 0-based
			URL        string         `json:"url,omitempty"`
			Exception  *cdpArg        `json:"exception,omitempty"`
			StackTrace *cdpStackTrace `json:"stackTrace,omitempty"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return raw
	}
	d := ev.ExceptionDetails
	text := d.Text
	if d.Exception != nil && d.Exception.Description != "" {
		text = d.Exception.Description
	}
	src := ""
	if d.URL != "" {
		src = fmt.Sprintf("%s:%d", shortFile(d.URL), d.LineNumber+1)
	} else {
		src = frameSrc(d.StackTrace)
	}
	return marshalEntry(consoleEntry{Text: text, Src: src}, raw)
}

// normalizeLogEntry converts a Log.entryAdded payload into a compact
// consoleEntry. Log.LogEntry.lineNumber is 1-based per CDP spec.
func normalizeLogEntry(raw json.RawMessage) json.RawMessage {
	var ev struct {
		Entry struct {
			Text       string `json:"text"`
			URL        string `json:"url,omitempty"`
			LineNumber int    `json:"lineNumber,omitempty"` // 1-based
		} `json:"entry"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return raw
	}
	src := ""
	if ev.Entry.URL != "" {
		src = fmt.Sprintf("%s:%d", shortFile(ev.Entry.URL), ev.Entry.LineNumber)
	}
	return marshalEntry(consoleEntry{Text: ev.Entry.Text, Src: src}, raw)
}

// argText renders a CDP RemoteObject arg as a human-readable string.
func argText(a cdpArg) string {
	switch a.Type {
	case "string":
		var s string
		if err := json.Unmarshal(a.Value, &s); err == nil {
			return s
		}
	case "undefined":
		return "undefined"
	case "object":
		if a.Subtype == "null" {
			return "null"
		}
		if a.Description != "" {
			return a.Description
		}
	case "function":
		if a.Description != "" {
			return a.Description
		}
	}
	if a.UnserializableValue != "" {
		return a.UnserializableValue
	}
	if len(a.Value) > 0 {
		return string(a.Value)
	}
	return a.Description
}

// frameSrc extracts "file.js:N" (1-based) from the first non-empty call frame.
func frameSrc(st *cdpStackTrace) string {
	if st == nil {
		return ""
	}
	for _, f := range st.CallFrames {
		if f.URL != "" {
			return fmt.Sprintf("%s:%d", shortFile(f.URL), f.LineNumber+1)
		}
	}
	return ""
}

// shortFile strips scheme, host, and query from a URL, returning the filename.
func shortFile(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		u = u[:i]
	}
	if i := strings.LastIndexByte(u, '/'); i >= 0 {
		return u[i+1:]
	}
	return u
}

func marshalEntry(e consoleEntry, fallback json.RawMessage) json.RawMessage {
	data, err := json.Marshal(e)
	if err != nil {
		return fallback
	}
	return data
}

// ensureNetworkPump wires Network.* events on cdpSessionID into a correlated
// ring buffer. Same idempotency contract as ensureConsolePump.
func ensureNetworkPump(ctx context.Context, env *tools.RunEnv, conn *cdpproto.Connection, cdpSID string, maxBuffer int) error {
	buf := env.EventBuf(session.NetworkBufKind, cdpSID, maxBuffer)
	if !env.MarkEventPumping(session.NetworkBufKind, cdpSID, maxBuffer) {
		return nil
	}
	if err := env.EnsureEnabled(ctx, cdpSID, "Network"); err != nil {
		return err
	}
	ch, _ := conn.SubscribeMethods([]string{
		"Network.requestWillBeSent",
		"Network.responseReceived",
		"Network.loadingFinished",
		"Network.loadingFailed",
	}, subscribeBuffer)
	go pumpNetwork(buf, cdpSID, ch)
	return nil
}

// pendingPumpCap caps the in-flight request map. Browsers occasionally never
// emit loadingFinished/loadingFailed (target navigates away mid-request), so
// without a cap the pump leaks memory until the connection drops.
const pendingPumpCap = 1000

type pendingRequest struct {
	url       string
	method    string
	resType   string
	reqHdrs   json.RawMessage
	respHdrs  json.RawMessage
	status    int
	statusTxt string
	mimeType  string
	size      int64
	t0        float64
	t1        float64
	hasResp   bool
}

type pendingFifo struct {
	order []string
	data  map[string]*pendingRequest
}

func newPendingFifo() *pendingFifo {
	return &pendingFifo{data: map[string]*pendingRequest{}}
}

func (f *pendingFifo) put(id string, p *pendingRequest) {
	if _, ok := f.data[id]; !ok {
		f.order = append(f.order, id)
	}
	f.data[id] = p
	if len(f.order) > pendingPumpCap {
		evict := f.order[0]
		f.order = f.order[1:]
		delete(f.data, evict)
	}
}

func (f *pendingFifo) take(id string) *pendingRequest {
	p, ok := f.data[id]
	if !ok {
		return nil
	}
	delete(f.data, id)
	for i, v := range f.order {
		if v == id {
			f.order = append(f.order[:i], f.order[i+1:]...)
			break
		}
	}
	return p
}

func (f *pendingFifo) peek(id string) *pendingRequest { return f.data[id] }

func pumpNetwork(buf *session.EventBuf, cdpSID string, ch <-chan cdpproto.Event) {
	pending := newPendingFifo()
	for ev := range ch {
		if ev.SessionID != cdpSID {
			continue
		}
		switch ev.Method {
		case "Network.requestWillBeSent":
			handleWillSend(pending, ev.Params)
		case "Network.responseReceived":
			handleRespRecv(pending, ev.Params)
		case "Network.loadingFinished":
			finalizeFinished(buf, pending, ev.Params)
		case "Network.loadingFailed":
			finalizeFailed(buf, pending, ev.Params)
		}
	}
}

func handleWillSend(p *pendingFifo, raw json.RawMessage) {
	var f struct {
		RequestID string `json:"requestId"`
		Request   struct {
			URL     string          `json:"url"`
			Method  string          `json:"method"`
			Headers json.RawMessage `json:"headers"`
		} `json:"request"`
		Type      string  `json:"type"`
		Timestamp float64 `json:"timestamp"`
	}
	if err := json.Unmarshal(raw, &f); err != nil || f.RequestID == "" {
		return
	}
	p.put(f.RequestID, &pendingRequest{
		url:     f.Request.URL,
		method:  f.Request.Method,
		resType: f.Type,
		reqHdrs: f.Request.Headers,
		t0:      f.Timestamp,
	})
}

func handleRespRecv(p *pendingFifo, raw json.RawMessage) {
	var f struct {
		RequestID string `json:"requestId"`
		Response  struct {
			Status     int             `json:"status"`
			StatusText string          `json:"statusText"`
			MimeType   string          `json:"mimeType"`
			Headers    json.RawMessage `json:"headers"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &f); err != nil || f.RequestID == "" {
		return
	}
	cur := p.peek(f.RequestID)
	if cur == nil {
		// CDP guarantees willSend precedes respRecv chronologically, but
		// our pump reads them via separate Subscribe channels so a select
		// can race. Materialize the entry so we don't drop the response.
		cur = &pendingRequest{}
		p.put(f.RequestID, cur)
	}
	cur.status = f.Response.Status
	cur.statusTxt = f.Response.StatusText
	cur.mimeType = f.Response.MimeType
	cur.respHdrs = f.Response.Headers
	cur.hasResp = true
}

type networkRecord struct {
	RequestID    string          `json:"requestId"`
	URL          string          `json:"url"`
	Method       string          `json:"method"`
	ResourceType string          `json:"resourceType,omitempty"`
	Status       int             `json:"status,omitempty"`
	StatusText   string          `json:"statusText,omitempty"`
	MimeType     string          `json:"mimeType,omitempty"`
	Size         int64           `json:"size,omitempty"`
	DurationMs   int64           `json:"durationMs,omitempty"`
	Failed       bool            `json:"failed,omitempty"`
	ErrorText    string          `json:"errorText,omitempty"`
	Canceled     bool            `json:"canceled,omitempty"`
	ReqHeaders   json.RawMessage `json:"requestHeaders,omitempty"`
	RespHeaders  json.RawMessage `json:"responseHeaders,omitempty"`
}

func finalizeFinished(buf *session.EventBuf, p *pendingFifo, raw json.RawMessage) {
	var f struct {
		RequestID         string  `json:"requestId"`
		Timestamp         float64 `json:"timestamp"`
		EncodedDataLength int64   `json:"encodedDataLength"`
	}
	if err := json.Unmarshal(raw, &f); err != nil || f.RequestID == "" {
		return
	}
	cur := p.take(f.RequestID)
	if cur == nil {
		// loadingFinished arrived before willSend (cross-channel race).
		// Emit the bare-minimum record rather than swallowing the event.
		cur = &pendingRequest{}
	}
	cur.size = f.EncodedDataLength
	cur.t1 = f.Timestamp
	rec := buildNetworkRecord(f.RequestID, cur, false, "", false)
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	buf.Append("network.complete", data)
}

func finalizeFailed(buf *session.EventBuf, p *pendingFifo, raw json.RawMessage) {
	var f struct {
		RequestID string  `json:"requestId"`
		Timestamp float64 `json:"timestamp"`
		ErrorText string  `json:"errorText"`
		Canceled  bool    `json:"canceled"`
	}
	if err := json.Unmarshal(raw, &f); err != nil || f.RequestID == "" {
		return
	}
	cur := p.take(f.RequestID)
	if cur == nil {
		// Failures sometimes arrive without a matching willSend (subresource
		// continuations, navigation aborts). Surface what we have.
		cur = &pendingRequest{}
	}
	cur.t1 = f.Timestamp
	rec := buildNetworkRecord(f.RequestID, cur, true, f.ErrorText, f.Canceled)
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	buf.Append("network.failed", data)
}

func buildNetworkRecord(reqID string, cur *pendingRequest, failed bool, errText string, canceled bool) networkRecord {
	rec := networkRecord{
		RequestID:    reqID,
		URL:          cur.url,
		Method:       cur.method,
		ResourceType: cur.resType,
		Status:       cur.status,
		StatusText:   cur.statusTxt,
		MimeType:     cur.mimeType,
		Size:         cur.size,
		Failed:       failed,
		ErrorText:    errText,
		Canceled:     canceled,
		ReqHeaders:   cur.reqHdrs,
		RespHeaders:  cur.respHdrs,
	}
	if cur.t1 > cur.t0 && cur.t0 > 0 {
		rec.DurationMs = int64((cur.t1 - cur.t0) * 1000)
	}
	return rec
}
