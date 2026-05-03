package mcp

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const fakeOKSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {"mode": {"type": "string", "enum": ["ok", "fail"]}},
  "required": ["mode"],
  "additionalProperties": false
}`

func newTestServer(t *testing.T) (*sdk.ClientSession, func()) {
	t.Helper()

	reg := tools.NewRegistry()
	reg.MustRegister(tools.Tool{
		Name:        "fake.run",
		Description: "test tool",
		Schema:      json.RawMessage(fakeOKSchema),
		NoSession:   true,
		Run: func(_ context.Context, raw json.RawMessage, _ *tools.RunEnv) tools.Result {
			var p struct {
				Mode string `json:"mode"`
			}
			_ = json.Unmarshal(raw, &p)
			switch p.Mode {
			case "ok":
				return tools.OK(map[string]any{"answer": 42})
			case "fail":
				return tools.Fail(tools.CategoryProtocol, "synthetic", "boom", false)
			}
			return tools.Fail(tools.CategoryInternal, "unknown_mode", p.Mode, false)
		},
	})

	mgr := session.NewManager(session.Config{})
	srv := NewServer(Options{
		Name:     "test",
		Version:  "v0.0.0",
		Registry: reg,
		Sessions: mgr,
	})

	ctx := context.Background()
	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.SDKServer().Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "client", Version: "v0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	cleanup := func() {
		_ = cs.Close()
		_ = ss.Close()
		mgr.Close()
	}
	return cs, cleanup
}

func TestListToolsAdvertisesRegisteredTool(t *testing.T) {
	cs, cleanup := newTestServer(t)
	defer cleanup()

	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var found *sdk.Tool
	for _, tl := range res.Tools {
		if tl.Name == "fake.run" {
			found = tl
		}
	}
	if found == nil {
		t.Fatalf("fake.run not in tools/list: %+v", res.Tools)
	}
	if found.Description != "test tool" {
		t.Errorf("description=%q, want %q", found.Description, "test tool")
	}
	// InputSchema should round-trip as a JSON object with type=object.
	raw, err := json.Marshal(found.InputSchema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("schema not an object: %v", err)
	}
	if asMap["type"] != "object" {
		t.Errorf("schema type=%v, want object", asMap["type"])
	}
}

func TestCallToolSuccessReturnsTextContentAndDiagnostics(t *testing.T) {
	cs, cleanup := newTestServer(t)
	defer cleanup()

	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "fake.run",
		Arguments: map[string]any{"mode": "ok"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %+v", res)
	}
	if len(res.Content) != 1 {
		t.Fatalf("len(Content)=%d, want 1", len(res.Content))
	}
	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("content type=%T, want *TextContent", res.Content[0])
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &data); err != nil {
		t.Fatalf("decode body: %v (body=%q)", err, tc.Text)
	}
	if got := data["answer"]; got != float64(42) {
		t.Errorf("answer=%v, want 42", got)
	}
	if res.Meta == nil {
		t.Fatalf("Meta nil, want diagnostics")
	}
	diag, ok := res.Meta[DiagnosticsMetaKey]
	if !ok {
		t.Fatalf("diagnostics meta missing; meta=%+v", res.Meta)
	}
	// On the wire, _meta values come back as map[string]any.
	dmap, ok := diag.(map[string]any)
	if !ok {
		t.Fatalf("diagnostics type=%T, want map", diag)
	}
	if dmap["tool"] != "fake.run" {
		t.Errorf("diagnostics.tool=%v, want fake.run", dmap["tool"])
	}
	if dmap["envelopeVersion"] != tools.EnvelopeVersion {
		t.Errorf("diagnostics.envelopeVersion=%v, want %s", dmap["envelopeVersion"], tools.EnvelopeVersion)
	}
}

func TestCallToolFailureSetsIsError(t *testing.T) {
	cs, cleanup := newTestServer(t)
	defer cleanup()

	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "fake.run",
		Arguments: map[string]any{"mode": "fail"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("IsError=false, want true")
	}
	tc := res.Content[0].(*sdk.TextContent)
	var ee tools.EnvelopeError
	if err := json.Unmarshal([]byte(tc.Text), &ee); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if ee.Code != "synthetic" || ee.Category != tools.CategoryProtocol {
		t.Errorf("err=%+v, want code=synthetic category=protocol", ee)
	}
}

func TestImageFromData_RoundTripsBase64(t *testing.T) {
	// page.screenshot returns {"mimeType":"image/png","data":"<base64>"}.
	// imageFromData must decode the base64 so the SDK doesn't double-encode.
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	data := map[string]any{"mimeType": "image/png", "data": png}
	img, ok := imageFromData(data)
	if !ok {
		t.Fatal("expected image detection to succeed")
	}
	if img.MIMEType != "image/png" {
		t.Errorf("mime: got %q want image/png", img.MIMEType)
	}
	if len(img.Data) == 0 {
		t.Error("expected decoded bytes, got empty")
	}
}

func TestImageFromData_RejectsNonImage(t *testing.T) {
	data := map[string]any{"mimeType": "application/json", "data": "abc"}
	if _, ok := imageFromData(data); ok {
		t.Error("expected non-image mime to be rejected")
	}
	if _, ok := imageFromData(map[string]any{"answer": 42}); ok {
		t.Error("expected unrelated payload to be rejected")
	}
}

func TestEnvelopeToResultPrependsSummaryBlock(t *testing.T) {
	// When a tool sets Summary, the adapter prepends a TextContent block
	// carrying the human-readable line ahead of the JSON payload. Chat
	// clients render that line in the OUT bubble.
	envOK := tools.Envelope{OK: true, Data: map[string]any{"answer": float64(42)}, Summary: "Returned 42."}
	got := envelopeToResult(envOK, false)
	if len(got.Content) != 2 {
		t.Fatalf("len(Content)=%d, want 2 (summary + payload)", len(got.Content))
	}
	first, ok := got.Content[0].(*sdk.TextContent)
	if !ok || first.Text != "Returned 42." {
		t.Errorf("first content=%+v, want TextContent{Returned 42.}", got.Content[0])
	}
	second, ok := got.Content[1].(*sdk.TextContent)
	if !ok {
		t.Fatalf("second content type=%T, want *TextContent", got.Content[1])
	}
	var asMap map[string]any
	if err := json.Unmarshal([]byte(second.Text), &asMap); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if asMap["answer"] != float64(42) {
		t.Errorf("payload.answer=%v, want 42", asMap["answer"])
	}

	// Failure with summary: IsError true, summary still leads the content.
	envErr := tools.Envelope{
		OK:      false,
		Error:   &tools.EnvelopeError{Code: "x", Message: "y", Category: tools.CategoryInternal},
		Summary: "Failed: y.",
	}
	gotErr := envelopeToResult(envErr, false)
	if !gotErr.IsError {
		t.Fatal("IsError=false, want true")
	}
	if len(gotErr.Content) != 2 {
		t.Fatalf("err len(Content)=%d, want 2", len(gotErr.Content))
	}
	if first, _ := gotErr.Content[0].(*sdk.TextContent); first == nil || first.Text != "Failed: y." {
		t.Errorf("error summary block missing; got=%+v", gotErr.Content[0])
	}
}

func TestEnvelopeToResultEmitsStructuredContent(t *testing.T) {
	// Tools that declare an OutputSchema get StructuredContent populated
	// alongside the JSON-encoded TextContent. Tools without OutputSchema
	// keep TextContent only.
	data := map[string]any{"answer": float64(42), "ok": true}
	envOK := tools.Envelope{OK: true, Data: data}

	withSchema := envelopeToResult(envOK, true)
	if withSchema.StructuredContent == nil {
		t.Errorf("StructuredContent nil with emitStructured=true")
	}
	asMap, ok := withSchema.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("StructuredContent type=%T, want map[string]any", withSchema.StructuredContent)
	}
	if asMap["answer"] != float64(42) {
		t.Errorf("StructuredContent.answer=%v, want 42", asMap["answer"])
	}
	// TextContent fallback should still be there.
	if len(withSchema.Content) != 1 {
		t.Fatalf("len(Content)=%d, want 1 (text fallback)", len(withSchema.Content))
	}

	withoutSchema := envelopeToResult(envOK, false)
	if withoutSchema.StructuredContent != nil {
		t.Errorf("StructuredContent=%v with emitStructured=false, want nil", withoutSchema.StructuredContent)
	}
}

func TestListToolsExposesAnnotationsAndOutputSchema(t *testing.T) {
	// Round-trip a tool with Title, Annotations, and OutputSchema through
	// the SDK's tools/list to make sure the adapter forwards each field.
	reg := tools.NewRegistry()
	reg.MustRegister(tools.Tool{
		Name:         "fake.annotated",
		Title:        "Fake Annotated",
		Description:  "exercise annotations + output schema",
		Schema:       json.RawMessage(`{"type":"object","additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"number"}}}`),
		Annotations: &tools.Annotations{
			ReadOnlyHint:    true,
			IdempotentHint:  true,
			DestructiveHint: tools.BoolPtr(false),
		},
		NoSession: true,
		Run: func(_ context.Context, _ json.RawMessage, _ *tools.RunEnv) tools.Result {
			return tools.OK(map[string]any{"x": float64(1)})
		},
	})
	mgr := session.NewManager(session.Config{})
	defer mgr.Close()
	srv := NewServer(Options{Name: "test", Version: "v0", Registry: reg, Sessions: mgr})

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

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var found *sdk.Tool
	for _, tl := range res.Tools {
		if tl.Name == "fake.annotated" {
			found = tl
		}
	}
	if found == nil {
		t.Fatalf("fake.annotated not in tools/list")
	}
	if found.Title != "Fake Annotated" {
		t.Errorf("Title=%q, want Fake Annotated", found.Title)
	}
	if found.OutputSchema == nil {
		t.Error("OutputSchema nil, want present")
	}
	if found.Annotations == nil || !found.Annotations.ReadOnlyHint {
		t.Errorf("Annotations.ReadOnlyHint not true; annotations=%+v", found.Annotations)
	}
}

func TestCallToolUnknownToolReturnsErrorEnvelope(t *testing.T) {
	cs, cleanup := newTestServer(t)
	defer cleanup()

	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "does.not.exist",
		Arguments: map[string]any{},
	})
	// Unknown tool is reported as a tool error (IsError) by the dispatcher,
	// not as a protocol error — the SDK only owns "tool not registered with
	// SDK" here, which we never hit because we don't register that name.
	if err == nil && !res.IsError {
		t.Fatalf("expected protocol or tool error, got %+v", res)
	}
}
