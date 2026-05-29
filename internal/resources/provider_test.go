package resources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

// stubRun is the per-tool behavior a test installs. It receives the raw params
// the Provider built from the URI so a test can assert dispatch routing.
type stubRun func(params json.RawMessage) tools.Result

// captured records the last (toolName, params) seen by any stub tool, so a
// Read/Fingerprint test can assert which tool the URI dispatched to and with
// what params.
type captured struct {
	tool   string
	params json.RawMessage
}

// newStubProvider builds a Provider backed by a real tools.Registry +
// Dispatcher. Every resource tool the Provider can dispatch is registered as a
// NoSession stub (so no CDP connection is required) whose Run delegates to fn.
// The returned *captured holds the most recent dispatch for assertions.
func newStubProvider(t *testing.T, fn stubRun) (*Provider, *captured) {
	t.Helper()
	reg := tools.NewRegistry()
	cap := &captured{}

	// Permissive schema: the Provider passes typed params (range string, folder
	// string, slide/limit ints) — accept any object.
	schema := json.RawMessage(`{"type":"object"}`)

	names := []string{
		"excel.tabulateRegion",
		"word.runScript",
		"outlook.query",
		"powerpoint.query",
		"onenote.query",
		"excel.discover",
		"word.discover",
		"outlook.discover",
		"powerpoint.discover",
		"onenote.discover",
	}
	for _, name := range names {
		name := name
		reg.MustRegister(tools.Tool{
			Name:      name,
			Schema:    schema,
			NoSession: true,
			Run: func(_ context.Context, params json.RawMessage, _ *tools.RunEnv) tools.Result {
				cap.tool = name
				cap.params = append([]byte(nil), params...)
				return fn(params)
			},
		})
	}

	disp := &tools.Dispatcher{Registry: reg}
	p := &Provider{
		Disp:     disp,
		Endpoint: func() webview2.Config { return webview2.Config{BrowserURL: "http://127.0.0.1:9222"} },
	}
	return p, cap
}

func TestProviderRead_Excel(t *testing.T) {
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"values": [][]any{{1, 2}}})
	})

	res, err := p.Read(context.Background(), "office://excel/Book1/Sheet1!A1:D20")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if cap.tool != "excel.tabulateRegion" {
		t.Errorf("dispatched tool = %q, want excel.tabulateRegion", cap.tool)
	}
	if res.MIMEType != "application/json" {
		t.Errorf("MIMEType = %q", res.MIMEType)
	}
	// The Excel reader uses the last part (Sheet1!A1:D20) as the range.
	var params map[string]any
	if err := json.Unmarshal(cap.params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["range"] != "Sheet1!A1:D20" {
		t.Errorf("range param = %v, want Sheet1!A1:D20", params["range"])
	}
	if !strings.Contains(res.Text, "values") {
		t.Errorf("Text missing payload: %q", res.Text)
	}
}

func TestProviderRead_ExcelSinglePart(t *testing.T) {
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"ok": true})
	})
	if _, err := p.Read(context.Background(), "office://excel/A1:B2"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	var params map[string]any
	if err := json.Unmarshal(cap.params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["range"] != "A1:B2" {
		t.Errorf("range param = %v, want A1:B2", params["range"])
	}
}

func TestProviderRead_ExcelNoParts(t *testing.T) {
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"ok": true})
	})
	if _, err := p.Read(context.Background(), "office://excel"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	var params map[string]any
	if err := json.Unmarshal(cap.params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["range"] != "" {
		t.Errorf("range param = %v, want empty", params["range"])
	}
}

func TestProviderRead_WordBody(t *testing.T) {
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"text": "hello"})
	})
	if _, err := p.Read(context.Background(), "office://word/mydoc"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if cap.tool != "word.runScript" {
		t.Fatalf("tool = %q, want word.runScript", cap.tool)
	}
	var params map[string]any
	if err := json.Unmarshal(cap.params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	script, _ := params["script"].(string)
	if !strings.Contains(script, "ctx.document.body") {
		t.Errorf("script missing body read: %q", script)
	}
	if strings.Contains(script, "getBookmarkRange") {
		t.Errorf("non-bookmark URI should not read a bookmark: %q", script)
	}
}

func TestProviderRead_WordBookmark(t *testing.T) {
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"text": "intro"})
	})
	if _, err := p.Read(context.Background(), "office://word/mydoc/bookmark/intro"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	var params map[string]any
	if err := json.Unmarshal(cap.params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	script, _ := params["script"].(string)
	if !strings.Contains(script, "getBookmarkRange") {
		t.Errorf("bookmark URI should inject getBookmarkRange: %q", script)
	}
	if !strings.Contains(script, `"intro"`) {
		t.Errorf("script should reference bookmark name: %q", script)
	}
}

func TestProviderRead_Outlook(t *testing.T) {
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"items": []any{}})
	})
	if _, err := p.Read(context.Background(), "office://outlook/inbox"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if cap.tool != "outlook.query" {
		t.Fatalf("tool = %q, want outlook.query", cap.tool)
	}
	var params map[string]any
	if err := json.Unmarshal(cap.params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["folder"] != "inbox" {
		t.Errorf("folder = %v, want inbox", params["folder"])
	}
	if params["limit"].(float64) != 50 {
		t.Errorf("limit = %v, want 50", params["limit"])
	}
}

func TestProviderRead_OutlookNoFolder(t *testing.T) {
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"items": []any{}})
	})
	if _, err := p.Read(context.Background(), "office://outlook"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	var params map[string]any
	if err := json.Unmarshal(cap.params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["folder"] != "" {
		t.Errorf("folder = %v, want empty", params["folder"])
	}
}

func TestProviderRead_PowerPointSlide(t *testing.T) {
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"slide": 2})
	})
	if _, err := p.Read(context.Background(), "office://pp/deck1/slide2"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if cap.tool != "powerpoint.query" {
		t.Fatalf("tool = %q, want powerpoint.query", cap.tool)
	}
	var params map[string]any
	if err := json.Unmarshal(cap.params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["slide"].(float64) != 2 {
		t.Errorf("slide = %v, want 2", params["slide"])
	}
}

func TestProviderRead_PowerPointDefaultSlide(t *testing.T) {
	// No "slideN" second part -> default slide 1.
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"slide": 1})
	})
	if _, err := p.Read(context.Background(), "office://pp/deck1"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	var params map[string]any
	if err := json.Unmarshal(cap.params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["slide"].(float64) != 1 {
		t.Errorf("slide = %v, want 1", params["slide"])
	}
}

func TestProviderRead_PowerPointNonSlidePart(t *testing.T) {
	// Second part present but not "slideN" -> stays at default 1.
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"ok": true})
	})
	if _, err := p.Read(context.Background(), "office://pp/deck1/notes"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	var params map[string]any
	if err := json.Unmarshal(cap.params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["slide"].(float64) != 1 {
		t.Errorf("slide = %v, want 1", params["slide"])
	}
}

func TestProviderRead_OneNote(t *testing.T) {
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"page": "x"})
	})
	if _, err := p.Read(context.Background(), "office://onenote/notebook/section/page"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if cap.tool != "onenote.query" {
		t.Fatalf("tool = %q, want onenote.query", cap.tool)
	}
	var params map[string]any
	if err := json.Unmarshal(cap.params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["limit"].(float64) != 1 {
		t.Errorf("limit = %v, want 1", params["limit"])
	}
}

func TestProviderRead_InvalidURI(t *testing.T) {
	p, _ := newStubProvider(t, func(json.RawMessage) tools.Result { return tools.OK(nil) })
	_, err := p.Read(context.Background(), "http://excel/Book1")
	if err == nil || !strings.Contains(err.Error(), "invalid URI") {
		t.Fatalf("want invalid URI error, got %v", err)
	}
}

func TestProviderRead_DispatchFailure(t *testing.T) {
	// Stub returns a failure -> Read maps to "tool dispatch failed".
	p, _ := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.Fail(tools.CategoryOfficeJS, "ItemNotFound", "Worksheet not found", false)
	})
	_, err := p.Read(context.Background(), "office://excel/Book1/Bad!A1")
	if err == nil || !strings.Contains(err.Error(), "tool dispatch failed") {
		t.Fatalf("want dispatch failed error, got %v", err)
	}
	if !strings.Contains(err.Error(), "Worksheet not found") {
		t.Errorf("error should carry the tool message: %v", err)
	}
}

func TestProviderRead_UnknownTool(t *testing.T) {
	// A registry missing the target tool surfaces the dispatcher's unknown_tool
	// error through the !env.OK branch.
	reg := tools.NewRegistry()
	disp := &tools.Dispatcher{Registry: reg}
	p := &Provider{
		Disp:     disp,
		Endpoint: func() webview2.Config { return webview2.Config{} },
	}
	_, err := p.Read(context.Background(), "office://excel/Book1/A1")
	if err == nil || !strings.Contains(err.Error(), "tool dispatch failed") {
		t.Fatalf("want dispatch failed error, got %v", err)
	}
}

func TestProviderRead_MarshalResultError(t *testing.T) {
	// A channel cannot be JSON-marshaled; the dispatcher stores it verbatim in
	// env.Data, so Read's json.Marshal(env.Data) fails -> "marshal result".
	p, _ := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(make(chan int))
	})
	_, err := p.Read(context.Background(), "office://excel/Book1/A1")
	if err == nil || !strings.Contains(err.Error(), "marshal result") {
		t.Fatalf("want marshal result error, got %v", err)
	}
}

func TestProviderFingerprint_OK(t *testing.T) {
	p, cap := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"filePath": "C:/Book1.xlsx", "fingerprint": "abc123"})
	})
	fp, err := p.Fingerprint(context.Background(), "office://excel/Book1")
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if fp != "abc123" {
		t.Errorf("fingerprint = %q, want abc123", fp)
	}
	if cap.tool != "excel.discover" {
		t.Errorf("tool = %q, want excel.discover", cap.tool)
	}
}

func TestProviderFingerprint_InvalidURI(t *testing.T) {
	p, _ := newStubProvider(t, func(json.RawMessage) tools.Result { return tools.OK(nil) })
	_, err := p.Fingerprint(context.Background(), "ftp://nope")
	if err == nil || !strings.Contains(err.Error(), "invalid URI") {
		t.Fatalf("want invalid URI error, got %v", err)
	}
}

func TestProviderFingerprint_DiscoverFailure(t *testing.T) {
	p, _ := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.Fail(tools.CategoryConnection, "session_dial_failed", "no excel", true)
	})
	_, err := p.Fingerprint(context.Background(), "office://excel/Book1")
	if err == nil || !strings.Contains(err.Error(), "discover failed") {
		t.Fatalf("want discover failed error, got %v", err)
	}
}

func TestProviderFingerprint_NonMapResult(t *testing.T) {
	// Discover returns a non-map (a slice) -> "unexpected result type".
	p, _ := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK([]any{"not", "a", "map"})
	})
	_, err := p.Fingerprint(context.Background(), "office://excel/Book1")
	if err == nil || !strings.Contains(err.Error(), "unexpected result type") {
		t.Fatalf("want unexpected result type error, got %v", err)
	}
}

func TestProviderFingerprint_MissingFingerprint(t *testing.T) {
	// Map present but no string "fingerprint" key -> "fingerprint not found".
	p, _ := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"filePath": "x"})
	})
	_, err := p.Fingerprint(context.Background(), "office://excel/Book1")
	if err == nil || !strings.Contains(err.Error(), "fingerprint not found") {
		t.Fatalf("want fingerprint not found error, got %v", err)
	}
}

func TestProviderFingerprint_FingerprintWrongType(t *testing.T) {
	// "fingerprint" present but not a string -> "fingerprint not found".
	p, _ := newStubProvider(t, func(json.RawMessage) tools.Result {
		return tools.OK(map[string]any{"fingerprint": 42})
	})
	_, err := p.Fingerprint(context.Background(), "office://excel/Book1")
	if err == nil || !strings.Contains(err.Error(), "fingerprint not found") {
		t.Fatalf("want fingerprint not found error, got %v", err)
	}
}
