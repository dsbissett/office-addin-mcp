package cdptool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/cdptool/generated"
)

// fakeScreenshotServer stands up a minimal CDP-shaped WebSocket that responds
// to Page.captureScreenshot with a fixed base64 payload. Other methods return
// an empty result; the test only cares about the screenshot path.
func fakeScreenshotServer(t *testing.T, payload []byte) (string, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.Close() }()
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			var f map[string]any
			if err := json.Unmarshal(data, &f); err != nil {
				continue
			}
			method, _ := f["method"].(string)
			result := map[string]any{}
			if method == "Page.captureScreenshot" {
				result["data"] = base64.StdEncoding.EncodeToString(payload)
			}
			_ = ws.WriteJSON(map[string]any{
				"id":     f["id"],
				"result": result,
			})
		}
	}))
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	return wsURL, srv.Close
}

// TestBinaryOutputPathWritesFile confirms a generated binary-field tool
// (Page.captureScreenshot) decodes the base64 result to disk when called
// with outputPath, and that the envelope returns BinaryOutput {path,
// sizeBytes, mimeType} instead of raw bytes.
//
// We bypass the dispatcher (no real session/target plumbing) by constructing
// a RunEnv whose Attach returns the same connection unchanged and whose
// EnsureEnabled is a no-op. That keeps the test focused on the codegen'd
// outputPath branch.
func TestBinaryOutputPathWritesFile(t *testing.T) {
	want := []byte("\x89PNG\r\n\x1a\nfake-screenshot-bytes")
	wsURL, stop := fakeScreenshotServer(t, want)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := cdp.Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	dest := filepath.Join(t.TempDir(), "screenshot.png")
	env := &tools.RunEnv{
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: conn, SessionID: "fake-cdp-session"}, nil
		},
		EnsureEnabled: func(context.Context, string, string) error { return nil },
	}
	params, _ := json.Marshal(map[string]any{"outputPath": dest})

	res := generated.NewPageCaptureScreenshot().Run(ctx, params, env)
	if res.Err != nil {
		t.Fatalf("run failed: %+v", res.Err)
	}

	got, ok := res.Data.(tools.BinaryOutput)
	if !ok {
		t.Fatalf("expected tools.BinaryOutput, got %T", res.Data)
	}
	if got.Path != dest {
		t.Errorf("path=%q want %q", got.Path, dest)
	}
	if got.MimeType != "image/png" {
		t.Errorf("mimeType=%q want image/png", got.MimeType)
	}
	if got.SizeBytes != int64(len(want)) {
		t.Errorf("sizeBytes=%d want %d", got.SizeBytes, len(want))
	}

	onDisk, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(onDisk) != string(want) {
		t.Errorf("file contents differ\ngot:  %q\nwant: %q", onDisk, want)
	}
}

// TestBinaryOutputPathOmittedReturnsRaw confirms that when outputPath is not
// supplied, the tool falls through to the standard json.RawMessage passthrough
// — outputPath gating shouldn't change behavior for callers who didn't ask
// for it.
func TestBinaryOutputPathOmittedReturnsRaw(t *testing.T) {
	want := []byte("raw-passthrough-bytes")
	wsURL, stop := fakeScreenshotServer(t, want)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := cdp.Dial(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	env := &tools.RunEnv{
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{Conn: conn, SessionID: "fake-cdp-session"}, nil
		},
		EnsureEnabled: func(context.Context, string, string) error { return nil },
	}

	res := generated.NewPageCaptureScreenshot().Run(ctx, []byte(`{}`), env)
	if res.Err != nil {
		t.Fatalf("run failed: %+v", res.Err)
	}
	if _, ok := res.Data.(tools.BinaryOutput); ok {
		t.Fatal("expected raw passthrough, got tools.BinaryOutput")
	}
	raw, ok := res.Data.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage passthrough, got %T", res.Data)
	}
	// The CDP result should still contain the base64-encoded data field.
	var probe struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	if probe.Data == "" {
		t.Error("raw passthrough lost the base64 data field")
	}
}
