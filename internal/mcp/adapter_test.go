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
