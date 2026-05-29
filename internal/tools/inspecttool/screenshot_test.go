package inspecttool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// pngBytes is a few bytes that base64-encode cleanly; the content is irrelevant
// to the tool, only the round-trip matters.
var pngBytes = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func TestRunScreenshot_HappyPNG(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
			if method == "Page.captureScreenshot" {
				var p struct {
					Format string `json:"format"`
				}
				if err := json.Unmarshal(params, &p); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if p.Format != "png" {
					t.Errorf("format=%q, want png (default)", p.Format)
				}
			}
			return map[string]any{"data": b64(pngBytes)}, nil
		},
	})
	res := runScreenshot(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out, ok := res.Data.(struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	})
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if out.MimeType != "image/png" || out.Data != b64(pngBytes) {
		t.Errorf("out=%+v", out)
	}
}

func TestRunScreenshot_JPEGWithQuality(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
			if method == "Page.captureScreenshot" {
				var p struct {
					Format  string `json:"format"`
					Quality int    `json:"quality"`
				}
				if err := json.Unmarshal(params, &p); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if p.Format != "jpeg" || p.Quality != 70 {
					t.Errorf("format/quality=%q/%d, want jpeg/70", p.Format, p.Quality)
				}
			}
			return map[string]any{"data": b64(pngBytes)}, nil
		},
	})
	res := runScreenshot(context.Background(), json.RawMessage(`{"format":"jpeg","quality":70}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	})
	if out.MimeType != "image/jpeg" {
		t.Errorf("mime=%q, want image/jpeg", out.MimeType)
	}
}

func TestRunScreenshot_OutputPathWritesFile(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "nested", "shot.png")
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return map[string]any{"data": b64(pngBytes)}, nil
		},
	})
	raw, err := json.Marshal(map[string]any{"outputPath": outPath})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res := runScreenshot(context.Background(), raw, env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	bin, ok := res.Data.(tools.BinaryOutput)
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if bin.Path != outPath || bin.SizeBytes != int64(len(pngBytes)) || bin.MimeType != "image/png" {
		t.Errorf("binary out=%+v", bin)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != string(pngBytes) {
		t.Errorf("file content mismatch")
	}
}

func TestRunScreenshot_OutputPathBadBase64(t *testing.T) {
	dir := t.TempDir()
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return map[string]any{"data": "not!!base64!!"}, nil
		},
	})
	raw, err := json.Marshal(map[string]any{"outputPath": filepath.Join(dir, "x.png")})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res := runScreenshot(context.Background(), raw, env)
	if res.Err == nil || res.Err.Code != "decode_base64" {
		t.Fatalf("want decode_base64, got %+v", res.Err)
	}
}

func TestRunScreenshot_EnablePageFailed(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		enableErr: errors.New("enable broke"),
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return map[string]any{"data": b64(pngBytes)}, nil
		},
	})
	res := runScreenshot(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "enable_page_failed" {
		t.Fatalf("want enable_page_failed, got %+v", res.Err)
	}
}

func TestRunScreenshot_CaptureFailed(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
			return nil, &cdp.RemoteError{Code: -32000, Message: "capture failed"}
		},
	})
	res := runScreenshot(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "capture_failed" {
		t.Fatalf("want capture_failed, got %+v", res.Err)
	}
}

func TestRunScreenshot_DecodeError(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			// "data" is a number, not a string → screenshot_decode fails.
			return map[string]any{"data": 12345}, nil
		},
	})
	res := runScreenshot(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "screenshot_decode" {
		t.Fatalf("want screenshot_decode, got %+v", res.Err)
	}
}

func TestRunScreenshot_AttachFailure(t *testing.T) {
	res := runScreenshot(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunScreenshot_BadParams(t *testing.T) {
	res := runScreenshot(context.Background(), json.RawMessage(`{"format":123}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

// --- clipFromUID via runScreenshot with a uid ---

func TestRunScreenshot_UIDClipHappy(t *testing.T) {
	target := cdp.TargetInfo{TargetID: "T-1"}
	sess := newSession()
	sess.SetSnapshot(&session.Snapshot{
		TargetID: "T-1",
		Nodes: map[string]session.SnapshotNode{
			"uid-1": {UID: "uid-1", BackendNodeID: 42, Role: "button"},
		},
	})
	var sawClip bool
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:   sess,
		target: target,
		resp: func(method string, params json.RawMessage) (any, *cdp.RemoteError) {
			switch method {
			case "DOM.getBoxModel":
				return map[string]any{
					"model": map[string]any{
						"content": []float64{10, 20, 110, 20, 110, 70, 10, 70},
						"width":   100,
						"height":  50,
					},
				}, nil
			case "Page.captureScreenshot":
				var p struct {
					Clip map[string]any `json:"clip"`
				}
				if err := json.Unmarshal(params, &p); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if p.Clip != nil {
					sawClip = true
					if p.Clip["x"] != float64(10) || p.Clip["y"] != float64(20) {
						t.Errorf("clip origin=%+v, want x=10 y=20", p.Clip)
					}
				}
				return map[string]any{"data": b64(pngBytes)}, nil
			}
			return map[string]any{}, nil
		},
	})
	res := runScreenshot(context.Background(), json.RawMessage(`{"uid":"uid-1"}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	if !sawClip {
		t.Errorf("expected a clip rect to be passed to captureScreenshot")
	}
}

func TestRunScreenshot_UIDNoSnapshotRuntime(t *testing.T) {
	// env.Snapshot is nil → no_snapshot_runtime.
	srvEnv := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			return &tools.AttachedTarget{SessionID: "cdp-1"}, nil
		},
		EnsureEnabled: func(context.Context, string, string) error { return nil },
		// Snapshot intentionally nil.
	}
	res := runScreenshot(context.Background(), json.RawMessage(`{"uid":"uid-1"}`), srvEnv)
	if res.Err == nil || res.Err.Code != "no_snapshot_runtime" {
		t.Fatalf("want no_snapshot_runtime, got %+v", res.Err)
	}
}

func TestRunScreenshot_UIDNoSnapshot(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	// fakeEnv installs Snapshot from a fresh session that has no snapshot set.
	res := runScreenshot(context.Background(), json.RawMessage(`{"uid":"uid-1"}`), env)
	if res.Err == nil || res.Err.Code != "no_snapshot" {
		t.Fatalf("want no_snapshot, got %+v", res.Err)
	}
}

func TestRunScreenshot_UIDSnapshotTargetMismatch(t *testing.T) {
	sess := newSession()
	sess.SetSnapshot(&session.Snapshot{TargetID: "OTHER", Nodes: map[string]session.SnapshotNode{}})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:   sess,
		target: cdp.TargetInfo{TargetID: "T-1"},
		resp:   func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runScreenshot(context.Background(), json.RawMessage(`{"uid":"uid-1"}`), env)
	if res.Err == nil || res.Err.Code != "snapshot_target_mismatch" {
		t.Fatalf("want snapshot_target_mismatch, got %+v", res.Err)
	}
}

func TestRunScreenshot_UIDNotFound(t *testing.T) {
	sess := newSession()
	sess.SetSnapshot(&session.Snapshot{TargetID: "T-1", Nodes: map[string]session.SnapshotNode{}})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:   sess,
		target: cdp.TargetInfo{TargetID: "T-1"},
		resp:   func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	res := runScreenshot(context.Background(), json.RawMessage(`{"uid":"missing"}`), env)
	if res.Err == nil || res.Err.Code != "uid_not_found" {
		t.Fatalf("want uid_not_found, got %+v", res.Err)
	}
}

func TestRunScreenshot_UIDEnableDOMFailed(t *testing.T) {
	sess := newSession()
	sess.SetSnapshot(&session.Snapshot{
		TargetID: "T-1",
		Nodes:    map[string]session.SnapshotNode{"uid-1": {UID: "uid-1", BackendNodeID: 7}},
	})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:      sess,
		target:    cdp.TargetInfo{TargetID: "T-1"},
		enableErr: errors.New("dom enable failed"),
		resp:      func(string, json.RawMessage) (any, *cdp.RemoteError) { return map[string]any{}, nil },
	})
	// EnsureEnabled fails for ALL domains here. The first call is Page (in
	// runScreenshot) which surfaces enable_page_failed before we reach the
	// DOM-enable inside clipFromUID. So this asserts the earliest enable error.
	res := runScreenshot(context.Background(), json.RawMessage(`{"uid":"uid-1"}`), env)
	if res.Err == nil || res.Err.Code != "enable_page_failed" {
		t.Fatalf("want enable_page_failed, got %+v", res.Err)
	}
}

func TestRunScreenshot_UIDGetBoxModelFailed(t *testing.T) {
	sess := newSession()
	sess.SetSnapshot(&session.Snapshot{
		TargetID: "T-1",
		Nodes:    map[string]session.SnapshotNode{"uid-1": {UID: "uid-1", BackendNodeID: 7}},
	})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:   sess,
		target: cdp.TargetInfo{TargetID: "T-1"},
		resp: func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
			if method == "DOM.getBoxModel" {
				return nil, &cdp.RemoteError{Code: -32000, Message: "no box"}
			}
			return map[string]any{"data": b64(pngBytes)}, nil
		},
	})
	res := runScreenshot(context.Background(), json.RawMessage(`{"uid":"uid-1"}`), env)
	if res.Err == nil || res.Err.Code != "get_box_model_failed" {
		t.Fatalf("want get_box_model_failed, got %+v", res.Err)
	}
}

func TestRunScreenshot_UIDBoxDecodeError(t *testing.T) {
	sess := newSession()
	sess.SetSnapshot(&session.Snapshot{
		TargetID: "T-1",
		Nodes:    map[string]session.SnapshotNode{"uid-1": {UID: "uid-1", BackendNodeID: 7}},
	})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:   sess,
		target: cdp.TargetInfo{TargetID: "T-1"},
		resp: func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
			if method == "DOM.getBoxModel" {
				// model is a string, not an object → box_decode.
				return map[string]any{"model": "garbage"}, nil
			}
			return map[string]any{"data": b64(pngBytes)}, nil
		},
	})
	res := runScreenshot(context.Background(), json.RawMessage(`{"uid":"uid-1"}`), env)
	if res.Err == nil || res.Err.Code != "box_decode" {
		t.Fatalf("want box_decode, got %+v", res.Err)
	}
}

func TestRunScreenshot_UIDBoxQuadTooShort(t *testing.T) {
	sess := newSession()
	sess.SetSnapshot(&session.Snapshot{
		TargetID: "T-1",
		Nodes:    map[string]session.SnapshotNode{"uid-1": {UID: "uid-1", BackendNodeID: 7}},
	})
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:   sess,
		target: cdp.TargetInfo{TargetID: "T-1"},
		resp: func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
			if method == "DOM.getBoxModel" {
				return map[string]any{
					"model": map[string]any{"content": []float64{1, 2, 3}, "width": 1, "height": 1},
				}, nil
			}
			return map[string]any{"data": b64(pngBytes)}, nil
		},
	})
	res := runScreenshot(context.Background(), json.RawMessage(`{"uid":"uid-1"}`), env)
	if res.Err == nil || res.Err.Code != "box_quad_invalid" {
		t.Fatalf("want box_quad_invalid, got %+v", res.Err)
	}
}

func TestScreenshot_ToolMetadata(t *testing.T) {
	tool := Screenshot()
	if tool.Name != "page.screenshot" || tool.Run == nil {
		t.Errorf("unexpected tool metadata: %+v", tool)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
}
