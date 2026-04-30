package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
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
	var env tools.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.OK {
		t.Fatal("expected ok=false")
	}
	if env.Error == nil || env.Error.Category != tools.CategoryNotFound {
		t.Fatalf("expected not_found error, got %+v", env.Error)
	}
	if env.Diagnostics.EnvelopeVersion != tools.EnvelopeVersion {
		t.Errorf("envelope version=%q, want %q",
			env.Diagnostics.EnvelopeVersion, tools.EnvelopeVersion)
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
	var env tools.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.OK {
		t.Fatal("expected ok=false")
	}
	if env.Error.Category != tools.CategoryValidation {
		t.Errorf("got category %q, want validation", env.Error.Category)
	}
}

// SchemaRejectsMissingRequiredField — the JSON Schema gates `expression` as
// required; this fails at the dispatcher boundary before the tool runs.
func TestRunCallSchemaRejectsMissingExpression(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.evaluate",
		"--param", `{}`,
		"--browser-url", "http://127.0.0.1:1",
	}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("got exit %d, want 1", code)
	}
	var env tools.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Code != "schema_violation" || env.Error.Category != tools.CategoryValidation {
		t.Fatalf("expected schema_violation/validation, got %+v", env.Error)
	}
}

// SchemaRejectsAdditionalProperty — additionalProperties:false in every
// schema. Detects accidental client typos before they hit CDP.
func TestRunCallSchemaRejectsAdditionalProperty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.evaluate",
		"--param", `{"expression":"1+1","oops":"typo"}`,
		"--browser-url", "http://127.0.0.1:1",
	}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("got exit %d", code)
	}
	var env tools.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Category != tools.CategoryValidation {
		t.Fatalf("expected validation, got %+v", env.Error)
	}
}

// SelectTargetSchemaRequiresSelector — selectTarget's anyOf requires at least
// one of targetId or urlPattern.
func TestRunCallSelectTargetRequiresSelector(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCall([]string{
		"--tool", "cdp.selectTarget",
		"--param", `{}`,
		"--browser-url", "http://127.0.0.1:1",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit=%d", code)
	}
	var env tools.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Category != tools.CategoryValidation {
		t.Errorf("expected validation, got %+v", env.Error)
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

var fakeWriteMu sync.Mutex

func writeWSJSON(t *testing.T, ws *websocket.Conn, v any) {
	t.Helper()
	fakeWriteMu.Lock()
	defer fakeWriteMu.Unlock()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := ws.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Logf("write (non-fatal, peer may have hung up): %v", err)
	}
}

type fakeTargets struct {
	infos []map[string]any
}

func (ft *fakeTargets) handle(t *testing.T, ws *websocket.Conn, f map[string]any) bool {
	method, _ := f["method"].(string)
	switch method {
	case "Target.getTargets":
		writeWSJSON(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"targetInfos": ft.infos},
		})
		return true
	case "Target.attachToTarget":
		writeWSJSON(t, ws, map[string]any{
			"id":     f["id"],
			"result": map[string]any{"sessionId": "fake-session"},
		})
		return true
	case "Target.createTarget":
		writeWSJSON(t, ws, map[string]any{
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

	var env tools.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK {
		t.Fatalf("err: %+v", env.Error)
	}
	if env.Diagnostics.Endpoint == "" {
		t.Errorf("expected endpoint diagnostic, got %+v", env.Diagnostics)
	}
	// Marshal env.Data into JSON so we can re-decode (since Data is `any`).
	dataBytes, _ := json.Marshal(env.Data)
	var data struct {
		Targets []map[string]any `json:"targets"`
	}
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %+v", data.Targets)
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
	var env tools.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
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
	var env tools.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Category != tools.CategoryNotFound {
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
			writeWSJSON(t, ws, map[string]any{
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
	var env tools.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Diagnostics.TargetID != "B" {
		t.Errorf("diag.targetId=%q, want B", env.Diagnostics.TargetID)
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
			writeWSJSON(t, ws, map[string]any{
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
	var env tools.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if !env.OK {
		t.Fatalf("err: %+v", env.Error)
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
			writeWSJSON(t, ws, map[string]any{
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
	var env tools.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env.Error == nil || env.Error.Code != "navigate_error" {
		t.Errorf("got %+v", env.Error)
	}
}

// list-tools: smoke test that the JSON document validates and contains the
// registered tools.
func TestRunListTools(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunListTools(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var out struct {
		EnvelopeVersion string `json:"envelopeVersion"`
		Tools           []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Schema      json.RawMessage `json:"schema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v\nout:%s", err, stdout.String())
	}
	if out.EnvelopeVersion != tools.EnvelopeVersion {
		t.Errorf("envelopeVersion=%q want %q", out.EnvelopeVersion, tools.EnvelopeVersion)
	}
	want := map[string]bool{
		"cdp.evaluate": false, "cdp.getTargets": false,
		"cdp.selectTarget": false, "browser.navigate": false,
		"excel.readRange": false, "excel.writeRange": false,
		"excel.listWorksheets": false, "excel.getActiveWorksheet": false,
		"excel.activateWorksheet": false, "excel.createWorksheet": false,
		"excel.deleteWorksheet": false, "excel.getSelectedRange": false,
		"excel.setSelectedRange": false, "excel.runScript": false,
		"excel.createTable": false,
	}
	for _, tt := range out.Tools {
		if _, ok := want[tt.Name]; ok {
			want[tt.Name] = true
		}
		if len(tt.Schema) == 0 {
			t.Errorf("tool %q missing schema", tt.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("tool %q not in list-tools output", name)
		}
	}
}

func TestRunListTools_RejectsArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunListTools([]string{"unexpected"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit=%d", code)
	}
}
