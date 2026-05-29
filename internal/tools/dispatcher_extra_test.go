package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/cdptest"
	"github.com/dsbissett/office-addin-mcp/internal/recorder"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

func TestNewDispatcher(t *testing.T) {
	reg := NewRegistry()
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	d := NewDispatcher(reg, mgr)
	if d.Registry != reg {
		t.Error("Registry not wired")
	}
	if d.Sessions != mgr {
		t.Error("Sessions not wired")
	}
	if d.Ephemeral {
		t.Error("NewDispatcher should leave Ephemeral false (daemon default)")
	}
}

func TestMarshalEnvelope(t *testing.T) {
	env := Envelope{
		OK:   true,
		Data: map[string]any{"answer": 42},
		Diagnostics: Diagnostics{
			Tool:            "fake.run",
			EnvelopeVersion: EnvelopeVersion,
		},
	}
	raw, err := MarshalEnvelope(env)
	if err != nil {
		t.Fatalf("MarshalEnvelope: %v", err)
	}
	var back Envelope
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !back.OK {
		t.Error("OK lost in round-trip")
	}
	if back.Diagnostics.Tool != "fake.run" {
		t.Errorf("Tool=%q", back.Diagnostics.Tool)
	}
}

func TestMarshalEnvelope_Error(t *testing.T) {
	// A channel value is not JSON-serializable; MarshalEnvelope must surface
	// the wrapped marshal error rather than panic.
	env := Envelope{OK: true, Data: make(chan int)}
	if _, err := MarshalEnvelope(env); err == nil {
		t.Fatal("expected marshal error for unserializable Data")
	}
}

func TestMustRegister_PanicsOnDuplicate(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(minimalTool("dup.tool"))
	defer func() {
		if recover() == nil {
			t.Fatal("MustRegister should panic on duplicate")
		}
	}()
	r.MustRegister(minimalTool("dup.tool"))
}

func TestSelectorCacheKey(t *testing.T) {
	cases := []struct {
		name string
		sel  TargetSelector
		want string
	}{
		{"empty", TargetSelector{}, ""},
		{"targetID only is empty key", TargetSelector{TargetID: "t1"}, ""},
		{"url pattern", TargetSelector{URLPattern: "taskpane"}, "taskpane"},
		{
			"surface only",
			TargetSelector{Surface: addin.SurfaceTaskpane},
			"surface=taskpane|addin=",
		},
		{
			"surface plus addin",
			TargetSelector{Surface: addin.SurfaceDialog, AddinID: "id-9"},
			"surface=dialog|addin=id-9",
		},
		{
			"addin id only",
			TargetSelector{AddinID: "id-9"},
			"surface=|addin=id-9",
		},
		{
			"url pattern wins over surface",
			TargetSelector{URLPattern: "pat", Surface: addin.SurfaceTaskpane},
			"pat",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := selectorCacheKey(tc.sel); got != tc.want {
				t.Errorf("selectorCacheKey(%+v)=%q want %q", tc.sel, got, tc.want)
			}
		})
	}
}

// liveDispatcher wires a Dispatcher whose sessions dial the in-process cdptest
// server, exercising the full session path: acquire → buildRunEnv → tool Run
// with a live Attach → finalize with real CDP round-trip accounting.
func liveDispatcher(t *testing.T, resp cdptest.Responder, reg *Registry) (*Dispatcher, webview2.Config) {
	t.Helper()
	srv := cdptest.NewServer(t, resp)
	mgr := session.NewManager(session.Config{ReconnectMax: 3, ReconnectWindow: time.Minute})
	t.Cleanup(mgr.Close)
	d := &Dispatcher{Registry: reg, Sessions: mgr, Ephemeral: true}
	return d, webview2.Config{WSEndpoint: srv.WSURL()}
}

func TestDispatch_SessionPath_AttachAndRoundTrips(t *testing.T) {
	reg := NewRegistry()
	// A session tool that resolves a target via Attach and reports the
	// resolved targetId back so the test can assert it threaded through.
	reg.MustRegister(Tool{
		Name:   "sess.attach",
		Schema: json.RawMessage(`{"type":"object"}`),
		Run: func(ctx context.Context, _ json.RawMessage, env *RunEnv) Result {
			att, err := env.Attach(ctx, TargetSelector{URLPattern: "taskpane"})
			if err != nil {
				return ClassifyCDPErr("attach", err)
			}
			return OK(map[string]any{
				"targetId":  att.Target.TargetID,
				"sessionId": att.SessionID,
			})
		},
	})

	d, ep := liveDispatcher(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "tp", Type: "page", URL: "https://example/taskpane.html"},
	}), reg)

	env := d.Dispatch(context.Background(), Request{Tool: "sess.attach", Endpoint: ep})
	if !env.OK {
		t.Fatalf("expected success, got %+v", env.Error)
	}
	data, _ := env.Data.(map[string]any)
	if data["targetId"] != "tp" {
		t.Errorf("targetId=%v want tp", data["targetId"])
	}
	if data["sessionId"] != "cdp-sess-1" {
		t.Errorf("sessionId=%v want cdp-sess-1", data["sessionId"])
	}
	// Endpoint diagnostic populated from the WS endpoint.
	if env.Diagnostics.Endpoint != ep.WSEndpoint {
		t.Errorf("diag.Endpoint=%q want %q", env.Diagnostics.Endpoint, ep.WSEndpoint)
	}
	if env.Diagnostics.TargetID != "tp" {
		t.Errorf("diag.TargetID=%q want tp", env.Diagnostics.TargetID)
	}
	if env.Diagnostics.CDPSessionID != "cdp-sess-1" {
		t.Errorf("diag.CDPSessionID=%q want cdp-sess-1", env.Diagnostics.CDPSessionID)
	}
	// At least getTargets + attachToTarget were issued.
	if env.Diagnostics.CDPRoundTrips < 2 {
		t.Errorf("CDPRoundTrips=%d want >=2", env.Diagnostics.CDPRoundTrips)
	}
}

func TestDispatch_SessionPath_BrowserURLEndpointDiagnostic(t *testing.T) {
	// When only BrowserURL is set the diagnostic falls back to it. We can't
	// dial a BrowserURL without a probe server, so assert the diagnostic on the
	// acquire-failure envelope (still finalized through the same code path).
	reg := NewRegistry()
	reg.MustRegister(Tool{
		Name:   "sess.noop",
		Schema: json.RawMessage(`{"type":"object"}`),
		Run:    func(context.Context, json.RawMessage, *RunEnv) Result { return OK(nil) },
	})
	mgr := session.NewManager(session.Config{ReconnectMax: 3, ReconnectWindow: time.Minute})
	defer mgr.Close()
	d := &Dispatcher{Registry: reg, Sessions: mgr, Ephemeral: true}

	ep := webview2.Config{BrowserURL: "http://127.0.0.1:1"}
	env := d.Dispatch(context.Background(), Request{Tool: "sess.noop", Endpoint: ep})
	if env.OK {
		t.Fatal("expected dial failure against dead BrowserURL")
	}
	if env.Error.Code != "session_dial_failed" {
		t.Errorf("code=%q want session_dial_failed", env.Error.Code)
	}
	if probed, _ := env.Error.Details["probedEndpoint"].(string); probed != ep.BrowserURL {
		t.Errorf("probedEndpoint=%q want %q", probed, ep.BrowserURL)
	}
}

func TestDispatch_SessionPath_OfficeJSEnrichment(t *testing.T) {
	// A session tool that returns an office_js failure; the dispatcher's
	// post-Run enrichment hook runs against the live session (Excel
	// ItemNotFound with no doccache → live listWorksheets lookup).
	reg := NewRegistry()
	reg.MustRegister(Tool{
		Name:      "excel.tabulateRegion",
		Schema:    json.RawMessage(`{"type":"object"}`),
		NoSession: false,
		Run: func(_ context.Context, _ json.RawMessage, _ *RunEnv) Result {
			return Fail(CategoryOfficeJS, "ItemNotFound", "sheet missing", false)
		},
	})

	resp := func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "Target.getTargets":
			return map[string]any{"targetInfos": []cdp.TargetInfo{
				{TargetID: "tp", Type: "page", URL: "https://example/taskpane.html"},
			}}, nil
		case "Target.attachToTarget":
			return map[string]any{"sessionId": "cdp-sess-1"}, nil
		case "Runtime.evaluate":
			return cdptest.EvalOffice(map[string]any{
				"worksheets": []map[string]any{{"name": "Inputs"}, {"name": "Outputs"}},
			}), nil
		default:
			return map[string]any{}, nil
		}
	}
	d, ep := liveDispatcher(t, resp, reg)

	env := d.Dispatch(context.Background(), Request{
		Tool:     "excel.tabulateRegion",
		Params:   json.RawMessage(`{"address":"Inputz!A1:B2"}`),
		Endpoint: ep,
	})
	if env.OK {
		t.Fatal("expected office_js failure")
	}
	sheets, _ := env.Error.Details["available_sheets"].([]string)
	if len(sheets) != 2 {
		t.Fatalf("available_sheets=%v want 2 live sheets", sheets)
	}
	if env.Error.Details["available_sheets_source"] != "live" {
		t.Errorf("available_sheets_source=%v want live", env.Error.Details["available_sheets_source"])
	}
}

func TestDispatch_NoSessionPath_OfficeJSEnrichmentNoAttach(t *testing.T) {
	// NoSession tool returning office_js: enrichment runs with env.Attach == nil
	// so live lookups are skipped, but address parsing still attaches details.
	reg := NewRegistry()
	reg.MustRegister(Tool{
		Name:      "excel.tabulateRegion",
		Schema:    json.RawMessage(`{"type":"object"}`),
		NoSession: true,
		Run: func(_ context.Context, _ json.RawMessage, _ *RunEnv) Result {
			return Fail(CategoryOfficeJS, "InvalidArgument", "bad range", false)
		},
	})
	env := Dispatch(context.Background(), reg, Request{
		Tool:   "excel.tabulateRegion",
		Params: json.RawMessage(`{"address":"Sheet1!A1:ZZZZ9"}`),
	})
	if env.OK {
		t.Fatal("expected office_js failure")
	}
	if env.Error.Details["column_out_of_bounds"] != "ZZZZ" {
		t.Errorf("column_out_of_bounds=%v want ZZZZ", env.Error.Details["column_out_of_bounds"])
	}
}

func TestDispatch_SessionPath_BuildRunEnvHelpers(t *testing.T) {
	// One session tool exercises the buildRunEnv closures: EnsureEnabled,
	// SetSnapshot/Snapshot, EventBuf, MarkEventPumping, SetDefaultSelection,
	// DefaultSelection (via a second empty-selector Attach hitting the sticky
	// default), the selector cache, ClearDefaultSelection, and Recording.
	reg := NewRegistry()
	reg.MustRegister(Tool{
		Name:   "sess.helpers",
		Schema: json.RawMessage(`{"type":"object"}`),
		Run: func(ctx context.Context, _ json.RawMessage, env *RunEnv) Result {
			conn, err := env.Conn(ctx)
			if err != nil {
				return ClassifyCDPErr("conn", err)
			}
			// First attach: resolves via getTargets + attachToTarget and caches.
			att, err := env.Attach(ctx, TargetSelector{URLPattern: "taskpane"})
			if err != nil {
				return ClassifyCDPErr("attach", err)
			}
			// Second attach with the same selector: cache hit, no new CDP calls.
			if _, err := env.Attach(ctx, TargetSelector{URLPattern: "taskpane"}); err != nil {
				return ClassifyCDPErr("attach2", err)
			}
			// EnsureEnabled issues a domain enable command.
			if err := env.EnsureEnabled(ctx, att.SessionID, "Runtime"); err != nil {
				return ClassifyCDPErr("ensure", err)
			}
			_ = conn
			// Snapshot round-trip.
			env.SetSnapshot(&session.Snapshot{TargetID: att.Target.TargetID, CDPSessionID: att.SessionID})
			if snap := env.Snapshot(); snap == nil || snap.TargetID != att.Target.TargetID {
				return Fail(CategoryInternal, "snap", "snapshot not stored", false)
			}
			// Event buffer get-or-create + pump reservation.
			if buf := env.EventBuf(session.ConsoleBufKind, att.SessionID, 16); buf == nil {
				return Fail(CategoryInternal, "evbuf", "nil event buffer", false)
			}
			if !env.MarkEventPumping(session.ConsoleBufKind, att.SessionID, 16) {
				return Fail(CategoryInternal, "pump", "first MarkEventPumping should win", false)
			}
			// Sticky default selection, then an empty-selector attach that hits it.
			env.SetDefaultSelection(att.Target, att.SessionID)
			def, err := env.Attach(ctx, TargetSelector{})
			if err != nil {
				return ClassifyCDPErr("attach-default", err)
			}
			if def.Target.TargetID != att.Target.TargetID {
				return Fail(CategoryInternal, "default", "empty selector did not hit sticky default", false)
			}
			env.ClearDefaultSelection()
			// Recording closure (non-nil because a Recorder is wired).
			if err := env.Recording("sess.helpers", []byte(`{}`)); err != nil {
				return ClassifyCDPErr("record", err)
			}
			return OK(map[string]any{"targetId": att.Target.TargetID})
		},
	})

	resp := func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
		switch method {
		case "Target.getTargets":
			return map[string]any{"targetInfos": []cdp.TargetInfo{
				{TargetID: "tp", Type: "page", URL: "https://example/taskpane.html"},
			}}, nil
		case "Target.attachToTarget":
			return map[string]any{"sessionId": "cdp-sess-1"}, nil
		default:
			return map[string]any{}, nil
		}
	}
	srv := cdptest.NewServer(t, resp)
	mgr := session.NewManager(session.Config{ReconnectMax: 3, ReconnectWindow: time.Minute})
	defer mgr.Close()
	recDir := t.TempDir()
	rec, err := recorder.New(recDir)
	if err != nil {
		t.Fatalf("recorder.New: %v", err)
	}
	if err := rec.StartRecording("macro1"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	d := &Dispatcher{Registry: reg, Sessions: mgr, Ephemeral: true, Recorder: rec}
	ep := webview2.Config{WSEndpoint: srv.WSURL()}

	env := d.Dispatch(context.Background(), Request{Tool: "sess.helpers", Endpoint: ep})
	if !env.OK {
		t.Fatalf("expected success, got %+v", env.Error)
	}
	data, _ := env.Data.(map[string]any)
	if data["targetId"] != "tp" {
		t.Errorf("targetId=%v want tp", data["targetId"])
	}
}

func TestClassifyAcquireErr_DefaultProbedEndpoint(t *testing.T) {
	// Empty config: probedEndpoint falls back to the well-known default port.
	got := classifyAcquireErr(session.ErrDialFailed, webview2.Config{})
	if probed, _ := got.Details["probedEndpoint"].(string); probed != "http://127.0.0.1:9222" {
		t.Errorf("probedEndpoint=%q want default", probed)
	}
}

func TestClassifyAcquireErr_WSEndpointProbed(t *testing.T) {
	got := classifyAcquireErr(session.ErrDialFailed, webview2.Config{WSEndpoint: "ws://x/y"})
	if probed, _ := got.Details["probedEndpoint"].(string); probed != "ws://x/y" {
		t.Errorf("probedEndpoint=%q want ws endpoint", probed)
	}
}

func TestDispatch_AutoRecover_SuccessfulRetry(t *testing.T) {
	// Initial endpoint is dead; Recover returns a live cdptest endpoint and the
	// retry succeeds. Also exercises the req.Log "auto-relaunched" warning.
	reg := NewRegistry()
	reg.MustRegister(Tool{
		Name:   "sess.op",
		Schema: json.RawMessage(`{"type":"object"}`),
		Run:    func(context.Context, json.RawMessage, *RunEnv) Result { return OK("recovered") },
	})
	srv := cdptest.NewServer(t, targetsResponder(t, []cdp.TargetInfo{
		{TargetID: "tp", Type: "page", URL: "https://example/taskpane.html"},
	}))
	live := webview2.Config{WSEndpoint: srv.WSURL()}

	mgr := session.NewManager(session.Config{ReconnectMax: 3, ReconnectWindow: time.Minute})
	defer mgr.Close()
	recoverCalls := 0
	d := &Dispatcher{
		Registry: reg,
		Sessions: mgr,
		Recover: func(context.Context) (webview2.Config, error) {
			recoverCalls++
			return live, nil
		},
	}

	var logLines []string
	env := d.Dispatch(context.Background(), Request{
		Tool:     "sess.op",
		Endpoint: webview2.Config{BrowserURL: "http://127.0.0.1:1"},
		Log:      func(_, message string) { logLines = append(logLines, message) },
	})
	if !env.OK {
		t.Fatalf("expected success after recovery, got %+v", env.Error)
	}
	if env.Data != "recovered" {
		t.Errorf("data=%v want recovered", env.Data)
	}
	if recoverCalls != 1 {
		t.Errorf("Recover called %d times, want 1", recoverCalls)
	}
	found := false
	for _, l := range logLines {
		if strings.Contains(l, "auto-relaunched") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected auto-relaunch log line, got %v", logLines)
	}
}

func TestNewRequestID_Unique(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 100; i++ {
		id := newRequestID()
		if len(id) != 16 {
			t.Fatalf("len(id)=%d want 16: %q", len(id), id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = struct{}{}
	}
}
