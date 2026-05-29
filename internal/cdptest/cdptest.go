// Package cdptest provides an in-process Chrome DevTools Protocol (CDP)
// WebSocket server for unit tests. It yields a real *cdp.Connection backed by
// a programmable Responder, so tool-package tests can exercise happy paths and
// Office.js error paths without a live Office host.
//
// Typical use from a tool package test:
//
//	srv := cdptest.NewServer(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
//		return cdptest.EvalOffice(map[string]any{"address": "A1"}), nil
//	})
//	env := &tools.RunEnv{
//		Diag: &tools.Diagnostics{},
//		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
//			return &tools.AttachedTarget{Conn: srv.Dial(t), SessionID: "cdp-1"}, nil
//		},
//	}
//	res := runSomeTool(context.Background(), json.RawMessage(`{}`), env)
package cdptest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
)

// Responder returns the reply for one inbound CDP command. A non-nil rerr makes
// the command fail with a CDP RemoteError; otherwise result is marshaled into
// the response "result" field. params is the raw command params object.
type Responder func(method string, params json.RawMessage) (result any, rerr *cdp.RemoteError)

// Server is an in-process CDP WebSocket server driven by a Responder.
type Server struct {
	srv   *httptest.Server
	wsURL string
}

// NewServer starts a CDP WebSocket server replying to every command via r. A
// nil responder replies with an empty result to everything. Cleanup is
// registered with t.
func NewServer(t testing.TB, r Responder) *Server {
	t.Helper()
	if r == nil {
		r = func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil }
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ws, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		serveConn(ws, r)
	}))
	s := &Server{srv: httpSrv, wsURL: "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/"}
	t.Cleanup(s.Close)
	return s
}

// serveConn reads frames and dispatches each through the responder until the
// socket closes.
func serveConn(ws *websocket.Conn, r Responder) {
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		reply, ok := buildReply(data, r)
		if !ok {
			continue
		}
		if err := ws.WriteMessage(websocket.TextMessage, reply); err != nil {
			return
		}
	}
}

// buildReply turns one inbound command frame into a response frame. ok is false
// when the inbound frame is not a command (no id) and should be ignored.
func buildReply(data []byte, r Responder) ([]byte, bool) {
	var in struct {
		ID     int64           `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(data, &in); err != nil || in.ID == 0 {
		return nil, false
	}
	result, rerr := r(in.Method, in.Params)
	out := map[string]any{"id": in.ID}
	if rerr != nil {
		out["error"] = rerr
	} else {
		out["result"] = result
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, false
	}
	return raw, true
}

// WSURL is the ws:// URL of the running server.
func (s *Server) WSURL() string { return s.wsURL }

// Dial opens a real *cdp.Connection to the server and registers cleanup with t.
func (s *Server) Dial(t testing.TB) *cdp.Connection {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := cdp.Dial(ctx, s.wsURL)
	if err != nil {
		t.Fatalf("cdptest dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// Close stops the server.
func (s *Server) Close() { s.srv.Close() }

// Eval builds the CDP result object for a Runtime.evaluate call whose
// returnByValue JS return value is value. Pass it from a Responder handling
// "Runtime.evaluate".
func Eval(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = []byte("null")
	}
	return map[string]any{
		"result": map[string]any{"type": "object", "value": json.RawMessage(raw)},
	}
}

// OfficeOK is the Office.js payload success envelope value: {"result": data}.
func OfficeOK(data any) any { return map[string]any{"result": data} }

// OfficeErr is the Office.js payload error envelope value. debugInfo may be nil.
func OfficeErr(code, message string, debugInfo any) any {
	m := map[string]any{"__officeError": true, "code": code, "message": message}
	if debugInfo != nil {
		m["debugInfo"] = debugInfo
	}
	return m
}

// EvalOffice is shorthand for a Runtime.evaluate result whose payload succeeded
// with data — i.e. Eval(OfficeOK(data)).
func EvalOffice(data any) any { return Eval(OfficeOK(data)) }

// EvalOfficeErr is shorthand for a Runtime.evaluate result whose payload
// signaled an Office.js error.
func EvalOfficeErr(code, message string, debugInfo any) any {
	return Eval(OfficeErr(code, message, debugInfo))
}
