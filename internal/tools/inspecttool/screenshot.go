package inspecttool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const screenshotSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.screenshot parameters",
  "type": "object",
  "properties": {
    "targetId":   {"type": "string"},
    "urlPattern": {"type": "string"},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"]},
    "uid":        {"type": "string", "description": "Snapshot UID to clip the screenshot to. Requires a recent page.snapshot."},
    "format":     {"type": "string", "enum": ["png", "jpeg"], "description": "Image format. Default png."},
    "quality":    {"type": "integer", "minimum": 0, "maximum": 100, "description": "JPEG quality 0–100. Ignored for png."},
    "outputPath": {"type": "string", "description": "If set, write the image to this path and return only metadata."}
  },
  "additionalProperties": false
}`

type screenshotParams struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
	Surface    string `json:"surface,omitempty"`
	UID        string `json:"uid,omitempty"`
	Format     string `json:"format,omitempty"`
	Quality    int    `json:"quality,omitempty"`
	OutputPath string `json:"outputPath,omitempty"`
}

// Screenshot returns the page.screenshot tool. It captures a PNG (or JPEG)
// of the active page, optionally clipped to the box-model of a snapshot UID.
// When outputPath is set the bytes are written to disk and only metadata is
// returned; otherwise base64 data rides back in the envelope.
func Screenshot() tools.Tool {
	return tools.Tool{
		Name:        "page.screenshot",
		Description: "Capture a screenshot of the active page, optionally clipped to a snapshot UID. With outputPath, writes the image to disk and returns metadata only.",
		Schema:      json.RawMessage(screenshotSchema),
		Annotations: &tools.Annotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: tools.BoolPtr(false)},
		Run:         runScreenshot,
	}
}

func runScreenshot(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p screenshotParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	format := screenshotFormat(p)

	att, attRes := attachForScreenshot(ctx, env, p)
	if attRes.Err != nil {
		return attRes
	}

	data, capRes := renderScreenshot(ctx, att, env, p, format)
	if capRes.Err != nil {
		return capRes
	}

	mime := screenshotMime(format)
	if p.OutputPath != "" {
		return writeScreenshot(p.OutputPath, data, format, mime)
	}
	return tools.OKWithSummary(
		fmt.Sprintf("Captured %s screenshot.", format),
		struct {
			MimeType string `json:"mimeType"`
			Data     string `json:"data"`
		}{MimeType: mime, Data: data},
	)
}

// screenshotFormat resolves the requested image format, defaulting to png.
func screenshotFormat(p screenshotParams) string {
	if p.Format == "" {
		return "png"
	}
	return p.Format
}

// screenshotMime maps an image format to its MIME type.
func screenshotMime(format string) string {
	if format == "jpeg" {
		return "image/jpeg"
	}
	return "image/png"
}

// attachForScreenshot resolves the target and enables the Page domain. A
// non-empty Result.Err signals an attach or enable failure to surface.
func attachForScreenshot(ctx context.Context, env *tools.RunEnv, p screenshotParams) (*tools.AttachedTarget, tools.Result) {
	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return nil, tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	if err := env.EnsureEnabled(ctx, att.SessionID, "Page"); err != nil {
		return nil, tools.ClassifyCDPErr("enable_page_failed", err)
	}
	return att, tools.Result{}
}

// renderScreenshot builds the capture args and performs the capture, returning
// the base64 image data. A non-empty Result.Err signals a clip, capture, or
// decode failure to surface.
func renderScreenshot(ctx context.Context, att *tools.AttachedTarget, env *tools.RunEnv, p screenshotParams, format string) (string, tools.Result) {
	args, argRes := screenshotArgs(ctx, att, env, p, format)
	if argRes.Err != nil {
		return "", argRes
	}
	return captureScreenshot(ctx, att, args)
}

// screenshotArgs builds the Page.captureScreenshot params, resolving the
// optional UID clip. A non-empty Result.Err signals a clip failure to surface.
func screenshotArgs(ctx context.Context, att *tools.AttachedTarget, env *tools.RunEnv, p screenshotParams, format string) (map[string]any, tools.Result) {
	args := map[string]any{"format": format}
	if format == "jpeg" && p.Quality > 0 {
		args["quality"] = p.Quality
	}
	if p.UID != "" {
		clip, res := clipFromUID(ctx, att, env, p.UID)
		if res.Err != nil {
			return nil, res
		}
		args["clip"] = clip
	}
	return args, tools.Result{}
}

// captureScreenshot sends Page.captureScreenshot and returns the base64 data.
// A non-empty Result.Err signals a capture or decode failure to surface.
func captureScreenshot(ctx context.Context, att *tools.AttachedTarget, args map[string]any) (string, tools.Result) {
	rawShot, err := att.Conn.Send(ctx, att.SessionID, "Page.captureScreenshot", args)
	if err != nil {
		return "", tools.ClassifyCDPErr("capture_failed", err)
	}
	var shot struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(rawShot, &shot); err != nil {
		return "", tools.Fail(tools.CategoryProtocol, "screenshot_decode", err.Error(), false)
	}
	return shot.Data, tools.Result{}
}

// writeScreenshot decodes the base64 image and writes it to outputPath,
// creating the parent directory if needed, then returns a metadata-only result.
func writeScreenshot(outputPath, data, format, mime string) tools.Result {
	bytes, decErr := base64.StdEncoding.DecodeString(data)
	if decErr != nil {
		return tools.Fail(tools.CategoryProtocol, "decode_base64", decErr.Error(), false)
	}
	if res := ensureOutputDir(outputPath); res.Err != nil {
		return res
	}
	if err := os.WriteFile(outputPath, bytes, 0o644); err != nil {
		return tools.Fail(tools.CategoryInternal, "output_write_failed", err.Error(), false)
	}
	return tools.OKWithSummary(
		fmt.Sprintf("Wrote %s screenshot (%d bytes) to %s.", format, len(bytes), outputPath),
		tools.BinaryOutput{
			Path:      outputPath,
			SizeBytes: int64(len(bytes)),
			MimeType:  mime,
		},
	)
}

// ensureOutputDir creates the parent directory of outputPath when it is a real
// subdirectory. A non-empty Result.Err signals a mkdir failure.
func ensureOutputDir(outputPath string) tools.Result {
	dir := filepath.Dir(outputPath)
	if dir == "." || dir == "" {
		return tools.Result{}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return tools.Fail(tools.CategoryInternal, "output_mkdir_failed", err.Error(), false)
	}
	return tools.Result{}
}

// clipFromUID asks DOM.getBoxModel for the snapshot node and converts its
// content quad into the rect Page.captureScreenshot expects.
func clipFromUID(ctx context.Context, att *tools.AttachedTarget, env *tools.RunEnv, uid string) (map[string]any, tools.Result) {
	node, res := resolveSnapshotNode(att, env, uid)
	if res.Err != nil {
		return nil, res
	}
	if err := env.EnsureEnabled(ctx, att.SessionID, "DOM"); err != nil {
		return nil, tools.ClassifyCDPErr("enable_dom_failed", err)
	}
	box, boxRes := fetchBoxModel(ctx, att, node.BackendNodeID)
	if boxRes.Err != nil {
		return nil, boxRes
	}
	return map[string]any{
		"x":      box.Content[0],
		"y":      box.Content[1],
		"width":  box.Width,
		"height": box.Height,
		"scale":  1,
	}, tools.Result{}
}

// resolveSnapshotNode validates the snapshot runtime/cache and looks up the
// node for uid on the current target. A non-empty Result.Err signals which
// precondition failed.
func resolveSnapshotNode(att *tools.AttachedTarget, env *tools.RunEnv, uid string) (session.SnapshotNode, tools.Result) {
	if env.Snapshot == nil {
		return session.SnapshotNode{}, tools.Fail(tools.CategoryUnsupported, "no_snapshot_runtime", "snapshot helper unavailable", false)
	}
	snap := env.Snapshot()
	if snap == nil {
		return session.SnapshotNode{}, tools.Fail(tools.CategoryNotFound, "no_snapshot", "call page.snapshot before passing uid", false)
	}
	if snap.TargetID != att.Target.TargetID {
		return session.SnapshotNode{}, tools.Fail(tools.CategoryNotFound, "snapshot_target_mismatch",
			fmt.Sprintf("snapshot was taken on target %s; current target is %s", snap.TargetID, att.Target.TargetID), false)
	}
	node, ok := snap.Nodes[uid]
	if !ok {
		return session.SnapshotNode{}, tools.Fail(tools.CategoryNotFound, "uid_not_found",
			fmt.Sprintf("uid %s not found in current snapshot", uid), false)
	}
	return node, tools.Result{}
}

// boxModel is the subset of DOM.getBoxModel we consume.
type boxModel struct {
	Content []float64
	Width   float64
	Height  float64
}

// fetchBoxModel fetches and validates the box model for a backend node. A
// non-empty Result.Err signals a CDP, decode, or quad-shape failure.
func fetchBoxModel(ctx context.Context, att *tools.AttachedTarget, backendNodeID int) (boxModel, tools.Result) {
	rawBox, err := att.Conn.Send(ctx, att.SessionID, "DOM.getBoxModel", map[string]any{
		"backendNodeId": backendNodeID,
	})
	if err != nil {
		return boxModel{}, tools.ClassifyCDPErr("get_box_model_failed", err)
	}
	var box struct {
		Model struct {
			Content []float64 `json:"content"`
			Width   float64   `json:"width"`
			Height  float64   `json:"height"`
		} `json:"model"`
	}
	if err := json.Unmarshal(rawBox, &box); err != nil {
		return boxModel{}, tools.Fail(tools.CategoryProtocol, "box_decode", err.Error(), false)
	}
	if len(box.Model.Content) < 8 {
		return boxModel{}, tools.Fail(tools.CategoryProtocol, "box_quad_invalid", "content quad too short", false)
	}
	return boxModel{Content: box.Model.Content, Width: box.Model.Width, Height: box.Model.Height}, tools.Result{}
}
