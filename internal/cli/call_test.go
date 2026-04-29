package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

func TestRunCallMissingTool(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall(nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("got exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--tool is required") {
		t.Errorf("missing usage hint, got %q", stderr.String())
	}
}

func TestRunCallUnknownTool(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall([]string{"--tool", "bogus.thing"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("got exit %d, want 1", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.OK {
		t.Fatal("expected ok=false")
	}
	if env.Error == nil || env.Error.Category != "not_found" {
		t.Fatalf("expected not_found error, got %+v", env.Error)
	}
}

func TestRunCallParamValidationFailsBeforeNetwork(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.evaluate",
		"--param", "{not json",
		"--browser-url", "http://127.0.0.1:1",
	}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("got exit %d, want 1", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.OK {
		t.Fatal("expected ok=false")
	}
	if env.Error.Category != "validation" {
		t.Errorf("got category %q, want validation", env.Error.Category)
	}
}

func TestRunCallMissingExpression(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.evaluate",
		"--param", `{}`,
		"--browser-url", "http://127.0.0.1:1",
	}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("got exit %d, want 1", code)
	}
	var env Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Code != "missing_expression" {
		t.Fatalf("expected missing_expression, got %+v", env.Error)
	}
}

// fakeBrowser combines an /json/version probe endpoint and a CDP-like WS at
// /ws. The handler is invoked for every inbound frame; it should write back
// the matching response (or events).
type fakeBrowser struct {
	*httptest.Server
}

func newFakeBrowser(t *testing.T, handle func(*testing.T, *websocket.Conn, map[string]any)) *fakeBrowser {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	var server *httptest.Server
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Browser":              "FakeBrowser/1.0",
			"webSocketDebuggerUrl": wsURL,
		})
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			var f map[string]any
			if err := json.Unmarshal(data, &f); err != nil {
				continue
			}
			handle(t, ws, f)
		}
	})
	server = httptest.NewServer(mux)
	return &fakeBrowser{Server: server}
}

func writeWSJSON(t *testing.T, ws *websocket.Conn, v any) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := ws.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Logf("write (non-fatal, peer may have hung up): %v", err)
	}
}

// writeMu serializes writes from the handler closure.
var fakeWriteMu sync.Mutex

func writeWSJSONLocked(t *testing.T, ws *websocket.Conn, v any) {
	fakeWriteMu.Lock()
	defer fakeWriteMu.Unlock()
	writeWSJSON(t, ws, v)
}

// fakeTargets supports getTargets, attachToTarget, createTarget responses for
// a configurable target list.
type fakeTargets struct {
	infos []map[string]any
}

func (ft *fakeTargets) handle(t *testing.T, ws *websocket.Conn, f map[string]any) bool {
	method, _ := f["method"].(string)
	switch method {
	case "Target.getTargets":
		writeWSJSONLocked(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"targetInfos": ft.infos},
		})
		return true
	case "Target.attachToTarget":
		writeWSJSONLocked(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"sessionId": "fake-session"},
		})
		return true
	case "Target.createTarget":
		writeWSJSONLocked(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"targetId": "created"},
		})
		return true
	}
	return false
}

func TestRunCall_GetTargets_FiltersInternalByDefault(t *testing.T) {
	ft := &fakeTargets{infos: []map[string]any{
		{"targetId": "A", "type": "page", "url": "https://app.example/main"},
		{"targetId": "B", "type": "page", "url": "chrome://newtab/"},
		{"targetId": "C", "type": "service_worker", "url": "https://app.example/sw.js"},
	}}
	fb := newFakeBrowser(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		ft.handle(t, ws, f)
	})
	defer fb.Close()

	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.getTargets",
		"--browser-url", fb.URL,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK {
		t.Fatalf("err: %+v", env.Error)
	}
	var data struct {
		Targets []map[string]any `json:"targets"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Targets) != 2 {
		t.Fatalf("expected 2 targets (page A + service_worker C kept; chrome:// dropped), got %+v", data.Targets)
	}
	if env.Diagnostics.Endpoint == "" {
		t.Errorf("expected endpoint diagnostic, got %+v", env.Diagnostics)
	}
}

func TestRunCall_GetTargets_TypeFilter(t *testing.T) {
	ft := &fakeTargets{infos: []map[string]any{
		{"targetId": "A", "type": "page", "url": "https://app.example/main"},
		{"targetId": "C", "type": "service_worker", "url": "https://app.example/sw.js"},
	}}
	fb := newFakeBrowser(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) { ft.handle(t, ws, f) })
	defer fb.Close()

	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.getTargets",
		"--param", `{"type":"page"}`,
		"--browser-url", fb.URL,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	var env Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	var data struct {
		Targets []map[string]any `json:"targets"`
	}
	_ = json.Unmarshal(env.Data, &data)
	if len(data.Targets) != 1 || data.Targets[0]["targetId"] != "A" {
		t.Errorf("got %+v", data.Targets)
	}
}

func TestRunCall_SelectTarget_RequiresSelector(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.selectTarget",
		"--param", `{}`,
		"--browser-url", "http://127.0.0.1:1",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d", code)
	}
	var env Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Code != "missing_selector" {
		t.Errorf("expected missing_selector, got %+v", env.Error)
	}
}

func TestRunCall_SelectTarget_ByURLPattern(t *testing.T) {
	ft := &fakeTargets{infos: []map[string]any{
		{"targetId": "A", "type": "page", "url": "https://app.example/main"},
		{"targetId": "B", "type": "page", "url": "https://app.example/addin/taskpane.html"},
	}}
	fb := newFakeBrowser(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) { ft.handle(t, ws, f) })
	defer fb.Close()

	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.selectTarget",
		"--param", `{"urlPattern":"taskpane"}`,
		"--browser-url", fb.URL,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var env Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	var data struct {
		Target map[string]any `json:"target"`
	}
	_ = json.Unmarshal(env.Data, &data)
	if data.Target["targetId"] != "B" {
		t.Errorf("got %+v", data.Target)
	}
	if env.Diagnostics.TargetID != "B" {
		t.Errorf("diag.targetId=%q, want B", env.Diagnostics.TargetID)
	}
}

func TestRunCall_SelectTarget_NotFound(t *testing.T) {
	ft := &fakeTargets{infos: []map[string]any{
		{"targetId": "A", "type": "page", "url": "https://app.example/main"},
	}}
	fb := newFakeBrowser(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) { ft.handle(t, ws, f) })
	defer fb.Close()

	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.selectTarget",
		"--param", `{"targetId":"ZZZ"}`,
		"--browser-url", fb.URL,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d", code)
	}
	var env Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Category != "not_found" {
		t.Errorf("expected not_found, got %+v", env.Error)
	}
}

func TestRunCall_Evaluate_TargetIDSelector(t *testing.T) {
	ft := &fakeTargets{infos: []map[string]any{
		{"targetId": "A", "type": "page", "url": "https://app.example/main"},
		{"targetId": "B", "type": "page", "url": "https://app.example/addin/taskpane.html"},
	}}
	fb := newFakeBrowser(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		if ft.handle(t, ws, f) {
			return
		}
		if f["method"] == "Runtime.evaluate" {
			writeWSJSONLocked(t, ws, map[string]any{
				"id": f["id"],
				"result": map[string]any{
					"result": map[string]any{
						"type":  "string",
						"value": "Excel",
					},
				},
			})
		}
	})
	defer fb.Close()

	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.evaluate",
		"--param", `{"expression":"globalThis.Office?.context?.host","targetId":"B"}`,
		"--browser-url", fb.URL,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var env Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Diagnostics.TargetID != "B" {
		t.Errorf("diag.targetId=%q, want B", env.Diagnostics.TargetID)
	}
	var data struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	_ = json.Unmarshal(env.Data, &data)
	if data.Type != "string" || string(data.Value) != `"Excel"` {
		t.Errorf("got %+v", data)
	}
}

func TestRunCall_Navigate(t *testing.T) {
	ft := &fakeTargets{infos: []map[string]any{
		{"targetId": "A", "type": "page", "url": "about:blank"},
	}}
	fb := newFakeBrowser(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		if ft.handle(t, ws, f) {
			return
		}
		if f["method"] == "Page.navigate" {
			writeWSJSONLocked(t, ws, map[string]any{
				"id":     f["id"],
				"result": map[string]any{"frameId": "F1", "loaderId": "L1"},
			})
		}
	})
	defer fb.Close()

	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "browser.navigate",
		"--param", `{"url":"https://example.com/app"}`,
		"--browser-url", fb.URL,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var env Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	var data struct {
		FrameID  string `json:"frameId"`
		LoaderID string `json:"loaderId"`
		URL      string `json:"url"`
	}
	_ = json.Unmarshal(env.Data, &data)
	if data.FrameID != "F1" || data.LoaderID != "L1" || data.URL != "https://example.com/app" {
		t.Errorf("got %+v", data)
	}
}

func TestRunCall_Navigate_ErrorText(t *testing.T) {
	ft := &fakeTargets{infos: []map[string]any{
		{"targetId": "A", "type": "page", "url": "about:blank"},
	}}
	fb := newFakeBrowser(t, func(t *testing.T, ws *websocket.Conn, f map[string]any) {
		if ft.handle(t, ws, f) {
			return
		}
		if f["method"] == "Page.navigate" {
			writeWSJSONLocked(t, ws, map[string]any{
				"id":     f["id"],
				"result": map[string]any{"frameId": "F1", "errorText": "ERR_NAME_NOT_RESOLVED"},
			})
		}
	})
	defer fb.Close()

	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "browser.navigate",
		"--param", `{"url":"https://nope.invalid/"}`,
		"--browser-url", fb.URL,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d", code)
	}
	var env Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Code != "navigate_error" {
		t.Errorf("got %+v", env.Error)
	}
}
