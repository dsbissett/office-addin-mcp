package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/doccache"
	"github.com/dsbissett/office-addin-mcp/internal/recorder"
	"github.com/dsbissett/office-addin-mcp/internal/resources"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// ---- setEndpoint / currentEndpoint -----------------------------------------

func TestSetEndpoint_RoundTrips(t *testing.T) {
	reg := tools.NewRegistry()
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	srv := NewServer(Options{
		Registry: reg,
		Sessions: mgr,
		Endpoint: webview2.Config{BrowserURL: "http://127.0.0.1:9222"},
		DocCache: doccache.Open("", true),
	})

	if got := srv.currentEndpoint(); got.BrowserURL != "http://127.0.0.1:9222" {
		t.Fatalf("initial currentEndpoint=%+v, want BrowserURL 9222", got)
	}

	srv.setEndpoint(webview2.Config{BrowserURL: "http://127.0.0.1:9333", WSEndpoint: "ws://x"})
	got := srv.currentEndpoint()
	if got.BrowserURL != "http://127.0.0.1:9333" || got.WSEndpoint != "ws://x" {
		t.Errorf("after setEndpoint currentEndpoint=%+v, want 9333/ws://x", got)
	}
}

// ---- setManifest / currentManifest -----------------------------------------

func TestSetManifest_RoundTrips(t *testing.T) {
	reg := tools.NewRegistry()
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	srv := NewServer(Options{Registry: reg, Sessions: mgr, DocCache: doccache.Open("", true)})

	if got := srv.currentManifest(); got != nil {
		t.Fatalf("initial currentManifest=%+v, want nil", got)
	}

	m := &addin.Manifest{Path: "C:/x/manifest.xml", DisplayName: "Demo", Hosts: []string{"Workbook"}}
	srv.setManifest(m)
	got := srv.currentManifest()
	if got == nil || got.DisplayName != "Demo" || got.Path != "C:/x/manifest.xml" {
		t.Errorf("after setManifest currentManifest=%+v, want Demo manifest", got)
	}
}

// ---- recoverConnection -----------------------------------------------------

// TestRecoverConnection_NoTrackedLaunch asserts the self-gating branch: with no
// tracked launch registered (the common case in tests; the MCP package never
// spawns a real Excel), recovery must refuse rather than relaunch anything.
// The probe-recovers and fresh-relaunch branches require a tracked launch,
// which can only be registered by launch.LaunchExcel spawning a real
// office-addin-debugging process — out of scope for unit tests.
func TestRecoverConnection_NoTrackedLaunch(t *testing.T) {
	reg := tools.NewRegistry()
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	// DisableAutoRecover is false so s.disp.Recover is wired to recoverConnection,
	// but we invoke the method directly to assert its self-gating behavior.
	srv := NewServer(Options{Registry: reg, Sessions: mgr, DocCache: doccache.Open("", true)})

	cfg, err := srv.recoverConnection(context.Background())
	if err == nil {
		t.Fatalf("expected error with no tracked launch, got cfg=%+v", cfg)
	}
	if !strings.Contains(err.Error(), "no tracked launch") {
		t.Errorf("err=%v, want 'no tracked launch'", err)
	}
	if cfg != (webview2.Config{}) {
		t.Errorf("cfg=%+v, want zero value on failure", cfg)
	}
}

// TestRecoverConnection_WiredWhenAutoRecoverEnabled confirms the constructor
// wires the recovery hook by default and leaves it nil when disabled.
func TestRecoverConnection_WiredWhenAutoRecoverEnabled(t *testing.T) {
	reg := tools.NewRegistry()
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()

	on := NewServer(Options{Registry: reg, Sessions: mgr, DocCache: doccache.Open("", true)})
	if on.disp.Recover == nil {
		t.Error("Recover hook nil with auto-recover enabled (default), want wired")
	}

	mgr2 := session.NewManager(session.Config{})
	defer mgr2.Close()
	off := NewServer(Options{Registry: reg, Sessions: mgr2, DisableAutoRecover: true, DocCache: doccache.Open("", true)})
	if off.disp.Recover != nil {
		t.Error("Recover hook wired with DisableAutoRecover=true, want nil")
	}
}

// ---- envelopeToResult marshal-failure branches -----------------------------

// unmarshalable carries a channel so json.Marshal fails, exercising the
// marshal-error branches of envelopeToResult and marshalFallback.
type unmarshalable struct {
	Ch chan int `json:"ch"`
}

func TestEnvelopeToResult_DataMarshalFailureUsesFallback(t *testing.T) {
	// Success envelope whose Data cannot be JSON-marshaled. imageFromData also
	// marshals first and returns (nil,false), so we land on json.Marshal(env.Data)
	// failing -> IsError + marshalFallback content.
	env := tools.Envelope{OK: true, Data: unmarshalable{Ch: make(chan int)}}
	got := envelopeToResult(env, false)
	if !got.IsError {
		t.Fatal("IsError=false, want true on data marshal failure")
	}
	if len(got.Content) != 1 {
		t.Fatalf("len(Content)=%d, want 1", len(got.Content))
	}
	tc, ok := got.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("content type=%T, want *TextContent", got.Content[0])
	}
	var ee tools.EnvelopeError
	if err := json.Unmarshal([]byte(tc.Text), &ee); err != nil {
		t.Fatalf("fallback not valid EnvelopeError JSON: %v (text=%q)", err, tc.Text)
	}
	if ee.Code != "marshal_failed" || ee.Category != tools.CategoryInternal {
		t.Errorf("fallback err=%+v, want marshal_failed/internal", ee)
	}
}

func TestEnvelopeToResult_ErrorMarshalFailureUsesFallback(t *testing.T) {
	// Failure envelope whose Error has unmarshalable Details, so
	// json.Marshal(env.Error) fails and the error branch uses marshalFallback.
	env := tools.Envelope{
		OK: false,
		Error: &tools.EnvelopeError{
			Code:     "x",
			Message:  "y",
			Category: tools.CategoryInternal,
			Details:  map[string]any{"bad": make(chan int)},
		},
	}
	got := envelopeToResult(env, false)
	if !got.IsError {
		t.Fatal("IsError=false, want true")
	}
	tc, ok := got.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("content type=%T, want *TextContent", got.Content[0])
	}
	if !strings.Contains(tc.Text, "marshal_failed") {
		t.Errorf("error fallback text=%q, want marshal_failed", tc.Text)
	}
}

func TestEnvelopeToResult_ImageBranchEmitsImageContent(t *testing.T) {
	// A success envelope whose Data looks like an inline image must produce an
	// ImageContent block (and any summary stays ahead of it).
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	env := tools.Envelope{
		OK:      true,
		Summary: "Captured screenshot.",
		Data:    map[string]any{"mimeType": "image/png", "data": png},
	}
	got := envelopeToResult(env, false)
	if got.IsError {
		t.Fatalf("IsError=true, want false: %+v", got)
	}
	if len(got.Content) != 2 {
		t.Fatalf("len(Content)=%d, want 2 (summary + image)", len(got.Content))
	}
	if first, _ := got.Content[0].(*sdk.TextContent); first == nil || first.Text != "Captured screenshot." {
		t.Errorf("first block=%+v, want summary TextContent", got.Content[0])
	}
	img, ok := got.Content[1].(*sdk.ImageContent)
	if !ok {
		t.Fatalf("second block type=%T, want *ImageContent", got.Content[1])
	}
	if img.MIMEType != "image/png" || len(img.Data) == 0 {
		t.Errorf("image=%+v, want decoded image/png bytes", img)
	}
	if got.StructuredContent != nil {
		t.Error("image envelope must not set StructuredContent")
	}
}

// ---- imageFromData base64-decode-failure -----------------------------------

func TestImageFromData_RejectsBadBase64(t *testing.T) {
	// image/* mime with non-empty but invalid base64 data must fail the decode
	// and return (nil,false) rather than emitting a corrupt image block.
	data := map[string]any{"mimeType": "image/png", "data": "not base64!!!"}
	if img, ok := imageFromData(data); ok {
		t.Errorf("expected bad base64 to be rejected, got %+v", img)
	}
}

func TestImageFromData_RejectsUnmarshalableProbe(t *testing.T) {
	// A value that cannot be marshaled at all must be rejected up front.
	if _, ok := imageFromData(unmarshalable{Ch: make(chan int)}); ok {
		t.Error("expected unmarshalable value to be rejected by imageFromData")
	}
}

// ---- marshalFallback (direct) ----------------------------------------------

func TestMarshalFallback_ProducesValidErrorJSON(t *testing.T) {
	out := marshalFallback(errInvalid{})
	var ee tools.EnvelopeError
	if err := json.Unmarshal([]byte(out), &ee); err != nil {
		t.Fatalf("marshalFallback output not valid JSON: %v (out=%q)", err, out)
	}
	if ee.Code != "marshal_failed" || ee.Category != tools.CategoryInternal || ee.Retryable {
		t.Errorf("fallback envelope=%+v, want marshal_failed/internal/non-retryable", ee)
	}
	if !strings.Contains(ee.Message, "boom") {
		t.Errorf("message=%q, want original error text", ee.Message)
	}
}

type errInvalid struct{}

func (errInvalid) Error() string { return "boom" }

// ---- resourceHandler -------------------------------------------------------

// newProvider builds a resources.Provider backed by a dispatcher over the given
// registry, with a fixed endpoint accessor. Used to exercise resourceHandler
// without a live Office host.
func newProvider(t *testing.T, reg *tools.Registry) *resources.Provider {
	t.Helper()
	mgr := session.NewManager(session.Config{})
	t.Cleanup(mgr.Close)
	disp := &tools.Dispatcher{Registry: reg, Sessions: mgr}
	return &resources.Provider{
		Disp:     disp,
		Endpoint: func() webview2.Config { return webview2.Config{} },
		Cache:    doccache.Open("", true),
	}
}

func TestResourceHandler_Success(t *testing.T) {
	// The excel resource maps office://excel/... -> excel.tabulateRegion. Register
	// a NoSession stand-in under that name so the dispatch succeeds without a CDP
	// connection, isolating resourceHandler's translation path.
	reg := tools.NewRegistry()
	reg.MustRegister(tools.Tool{
		Name:      "excel.tabulateRegion",
		Schema:    json.RawMessage(`{"type":"object","additionalProperties":true}`),
		NoSession: true,
		Run: func(_ context.Context, raw json.RawMessage, _ *tools.RunEnv) tools.Result {
			var p map[string]any
			if err := json.Unmarshal(raw, &p); err != nil {
				return tools.Fail(tools.CategoryValidation, "bad", err.Error(), false)
			}
			return tools.OK(map[string]any{"range": p["range"], "rows": []any{}})
		},
	})
	provider := newProvider(t, reg)

	req := &sdk.ReadResourceRequest{Params: &sdk.ReadResourceParams{URI: "office://excel/Book1/Sheet1!A1:B2"}}
	res, err := resourceHandler(context.Background(), req, provider)
	if err != nil {
		t.Fatalf("resourceHandler: %v", err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("len(Contents)=%d, want 1", len(res.Contents))
	}
	c := res.Contents[0]
	if c.URI != "office://excel/Book1/Sheet1!A1:B2" {
		t.Errorf("URI=%q, want round-trip", c.URI)
	}
	if c.MIMEType != "application/json" {
		t.Errorf("MIMEType=%q, want application/json", c.MIMEType)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(c.Text), &payload); err != nil {
		t.Fatalf("content text not JSON: %v (text=%q)", err, c.Text)
	}
	if payload["range"] != "Sheet1!A1:B2" {
		t.Errorf("payload range=%v, want Sheet1!A1:B2", payload["range"])
	}
}

func TestResourceHandler_NotFoundOnBadURI(t *testing.T) {
	// A malformed URI makes provider.Read fail; resourceHandler must surface a
	// ResourceNotFound error (nil result).
	provider := newProvider(t, tools.NewRegistry())
	req := &sdk.ReadResourceRequest{Params: &sdk.ReadResourceParams{URI: "not-an-office-uri"}}
	res, err := resourceHandler(context.Background(), req, provider)
	if err == nil {
		t.Fatalf("expected error for bad URI, got res=%+v", res)
	}
	if res != nil {
		t.Errorf("res=%+v, want nil on error", res)
	}
}

func TestResourceHandler_NotFoundOnDispatchFailure(t *testing.T) {
	// Valid URI but the mapped tool isn't registered, so the dispatch fails and
	// resourceHandler returns ResourceNotFound.
	provider := newProvider(t, tools.NewRegistry())
	req := &sdk.ReadResourceRequest{Params: &sdk.ReadResourceParams{URI: "office://excel/Book1/Sheet1!A1"}}
	res, err := resourceHandler(context.Background(), req, provider)
	if err == nil {
		t.Fatalf("expected error when mapped tool missing, got res=%+v", res)
	}
	if res != nil {
		t.Errorf("res=%+v, want nil on error", res)
	}
}

// ---- createMacroReplayTool -------------------------------------------------

func TestCreateMacroReplayTool_ReplaysThroughDispatcher(t *testing.T) {
	// The replay tool dispatches each recorded step through the dispatcher. Wire a
	// NoSession tool the macro will replay and assert the macro's Run drives it.
	reg := tools.NewRegistry()
	var dispatched []string
	reg.MustRegister(tools.Tool{
		Name:      "excel.runScript",
		Schema:    json.RawMessage(`{"type":"object","additionalProperties":true}`),
		NoSession: true,
		Run: func(_ context.Context, raw json.RawMessage, _ *tools.RunEnv) tools.Result {
			dispatched = append(dispatched, string(raw))
			return tools.OK(map[string]any{"ran": true})
		},
	})
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	disp := &tools.Dispatcher{Registry: reg, Sessions: mgr}

	macro := &recorder.Macro{
		Name: "demo",
		Entries: []recorder.Entry{
			{Tool: "excel.runScript", Params: map[string]any{"script": "one"}},
			{Tool: "excel.runScript", Params: map[string]any{"script": "two"}},
		},
	}
	tool := createMacroReplayTool(macro, disp)
	if tool.Name != "macro.demo" {
		t.Fatalf("tool name=%q, want macro.demo", tool.Name)
	}

	env := &tools.RunEnv{Diag: &tools.Diagnostics{}}
	res := tool.Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("replay failed: %+v", res.Err)
	}
	if len(dispatched) != 2 {
		t.Fatalf("dispatched %d steps, want 2: %v", len(dispatched), dispatched)
	}
	data, ok := res.Data.(map[string]any)
	if !ok || data["macro"] != "demo" || data["stepsReplayed"] != 2 {
		t.Errorf("replay data=%v, want demo/2", res.Data)
	}
}

func TestCreateMacroReplayTool_PropagatesStepFailure(t *testing.T) {
	// When a dispatched step fails, the replay tool surfaces the failure (not OK).
	reg := tools.NewRegistry()
	reg.MustRegister(tools.Tool{
		Name:      "excel.runScript",
		Schema:    json.RawMessage(`{"type":"object","additionalProperties":true}`),
		NoSession: true,
		Run: func(_ context.Context, _ json.RawMessage, _ *tools.RunEnv) tools.Result {
			return tools.Fail(tools.CategoryOfficeJS, "ItemNotFound", "nope", true)
		},
	})
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	disp := &tools.Dispatcher{Registry: reg, Sessions: mgr}

	macro := &recorder.Macro{
		Name:    "fails",
		Entries: []recorder.Entry{{Tool: "excel.runScript", Params: map[string]any{"script": "x"}}},
	}
	res := createMacroReplayTool(macro, disp).Run(context.Background(), json.RawMessage(`{}`), &tools.RunEnv{Diag: &tools.Diagnostics{}})
	if res.Err == nil {
		t.Fatal("expected failure to propagate from dispatched step")
	}
	if res.Err.Code != "ItemNotFound" {
		t.Errorf("err code=%q, want ItemNotFound", res.Err.Code)
	}
}

func TestCreateMacroReplayTool_ForwardsEndpoint(t *testing.T) {
	// When the RunEnv carries an explicit endpoint, the sub-dispatch must inherit
	// it (covers the Endpoint-forwarding branch in the runner closure).
	reg := tools.NewRegistry()
	var sawEndpoint webview2.Config
	reg.MustRegister(tools.Tool{
		Name:      "excel.runScript",
		Schema:    json.RawMessage(`{"type":"object","additionalProperties":true}`),
		NoSession: true,
		Run: func(_ context.Context, _ json.RawMessage, env *tools.RunEnv) tools.Result {
			sawEndpoint = env.Endpoint
			return tools.OK(nil)
		},
	})
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	disp := &tools.Dispatcher{Registry: reg, Sessions: mgr}

	macro := &recorder.Macro{
		Name:    "ep",
		Entries: []recorder.Entry{{Tool: "excel.runScript", Params: map[string]any{}}},
	}
	env := &tools.RunEnv{
		Diag:     &tools.Diagnostics{},
		Endpoint: webview2.Config{BrowserURL: "http://127.0.0.1:9222"},
	}
	res := createMacroReplayTool(macro, disp).Run(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("replay failed: %+v", res.Err)
	}
	if sawEndpoint.BrowserURL != "http://127.0.0.1:9222" {
		t.Errorf("sub-dispatch endpoint=%+v, want forwarded BrowserURL 9222", sawEndpoint)
	}
}

// ---- NewServer with recorder (macro auto-registration) ----------------------

func TestNewServer_RegistersLoadedMacros(t *testing.T) {
	// A recorder with a persisted macro must have a corresponding macro.* replay
	// tool registered on the server's registry at construction time.
	store, err := recorder.New(t.TempDir())
	if err != nil {
		t.Fatalf("recorder.New: %v", err)
	}
	if err := store.StartRecording("saved"); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if err := store.Append("excel.runScript", json.RawMessage(`{"script":"x"}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := store.StopRecording(); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}

	reg := tools.NewRegistry()
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	srv := NewServer(Options{
		Registry: reg,
		Sessions: mgr,
		Recorder: store,
		DocCache: doccache.Open("", true),
	})
	_ = srv

	if _, ok := reg.Get("macro.saved"); !ok {
		t.Errorf("macro.saved replay tool not registered from recorder; tools=%v", reg.List())
	}
}

func TestNewServer_NilRecorderSkipsMacroRegistration(t *testing.T) {
	// With no recorder, no macro.* tools are added — guards the nil branch.
	reg := tools.NewRegistry()
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	NewServer(Options{Registry: reg, Sessions: mgr, DocCache: doccache.Open("", true)})
	for _, tl := range reg.List() {
		if strings.HasPrefix(tl.Name, "macro.") {
			t.Errorf("unexpected macro tool %q registered without a recorder", tl.Name)
		}
	}
}

// ---- NewServer defaults (Sessions/DocCache nil branches) -------------------

func TestNewServer_DefaultsSessionsAndDocCache(t *testing.T) {
	// nil Sessions and nil DocCache must be filled in by the constructor without
	// panicking; the server is usable (SDKServer wired).
	reg := tools.NewRegistry()
	srv := NewServer(Options{Registry: reg})
	if srv.SDKServer() == nil {
		t.Fatal("SDKServer nil after construction with defaulted Sessions/DocCache")
	}
	if srv.disp.Sessions == nil {
		t.Error("dispatcher Sessions nil, want a default manager")
	}
	if srv.disp.DocCache == nil {
		t.Error("dispatcher DocCache nil, want a default store")
	}
	srv.disp.Sessions.Close()
}

func TestNewServer_PanicsWithoutRegistry(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when Registry is nil")
		}
	}()
	NewServer(Options{})
}

func TestNewServer_DefaultsNameAndVersion(t *testing.T) {
	reg := tools.NewRegistry()
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	// Empty Name/Version must be defaulted; verify the SDK reports the fallback
	// implementation name on initialize.
	srv := NewServer(Options{Registry: reg, Sessions: mgr, DocCache: doccache.Open("", true)})

	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.SDKServer().Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = ss.Close() }()
	client := sdk.NewClient(&sdk.Implementation{Name: "client", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	init := cs.InitializeResult()
	if init == nil || init.ServerInfo == nil {
		t.Fatalf("no initialize result/server info: %+v", init)
	}
	if init.ServerInfo.Name != "office-addin-mcp" {
		t.Errorf("server name=%q, want defaulted office-addin-mcp", init.ServerInfo.Name)
	}
	if init.ServerInfo.Version != "0.0.0-dev" {
		t.Errorf("server version=%q, want defaulted 0.0.0-dev", init.ServerInfo.Version)
	}
}

// ---- Run -------------------------------------------------------------------

// TestRun_ReturnsOnCanceledContext drives Server.Run with an already-canceled
// context. StdioTransport.Connect doesn't block (it just wraps os.Stdin/Stdout),
// so the SDK's run loop immediately observes the canceled context, closes the
// session, and returns ctx.Err() wrapped as "mcp serve". This exercises Run's
// happy wiring + error-wrap without a live peer.
func TestRun_ReturnsOnCanceledContext(t *testing.T) {
	reg := tools.NewRegistry()
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	srv := NewServer(Options{Registry: reg, Sessions: mgr, DocCache: doccache.Open("", true)})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := srv.Run(ctx)
	if err == nil {
		t.Fatal("Run returned nil on canceled context, want wrapped error")
	}
	if !strings.Contains(err.Error(), "mcp serve") {
		t.Errorf("err=%v, want wrapped 'mcp serve'", err)
	}
}
