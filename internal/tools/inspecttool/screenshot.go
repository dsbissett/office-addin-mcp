package inspecttool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
		Run:         runScreenshot,
	}
}

func runScreenshot(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p screenshotParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	format := p.Format
	if format == "" {
		format = "png"
	}

	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}
	if err := env.EnsureEnabled(ctx, att.SessionID, "Page"); err != nil {
		return tools.ClassifyCDPErr("enable_page_failed", err)
	}

	args := map[string]any{"format": format}
	if format == "jpeg" && p.Quality > 0 {
		args["quality"] = p.Quality
	}

	if p.UID != "" {
		clip, res := clipFromUID(ctx, att, env, p.UID)
		if res.Err != nil {
			return res
		}
		args["clip"] = clip
	}

	rawShot, err := att.Conn.Send(ctx, att.SessionID, "Page.captureScreenshot", args)
	if err != nil {
		return tools.ClassifyCDPErr("capture_failed", err)
	}
	var shot struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(rawShot, &shot); err != nil {
		return tools.Fail(tools.CategoryProtocol, "screenshot_decode", err.Error(), false)
	}

	mime := "image/png"
	if format == "jpeg" {
		mime = "image/jpeg"
	}

	if p.OutputPath != "" {
		bytes, decErr := base64.StdEncoding.DecodeString(shot.Data)
		if decErr != nil {
			return tools.Fail(tools.CategoryProtocol, "decode_base64", decErr.Error(), false)
		}
		if dir := filepath.Dir(p.OutputPath); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return tools.Fail(tools.CategoryInternal, "output_mkdir_failed", err.Error(), false)
			}
		}
		if err := os.WriteFile(p.OutputPath, bytes, 0o644); err != nil {
			return tools.Fail(tools.CategoryInternal, "output_write_failed", err.Error(), false)
		}
		return tools.OK(tools.BinaryOutput{
			Path:      p.OutputPath,
			SizeBytes: int64(len(bytes)),
			MimeType:  mime,
		})
	}

	return tools.OK(struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	}{MimeType: mime, Data: shot.Data})
}

// clipFromUID asks DOM.getBoxModel for the snapshot node and converts its
// content quad into the rect Page.captureScreenshot expects.
func clipFromUID(ctx context.Context, att *tools.AttachedTarget, env *tools.RunEnv, uid string) (map[string]any, tools.Result) {
	if env.Snapshot == nil {
		return nil, tools.Fail(tools.CategoryUnsupported, "no_snapshot_runtime", "snapshot helper unavailable", false)
	}
	snap := env.Snapshot()
	if snap == nil {
		return nil, tools.Fail(tools.CategoryNotFound, "no_snapshot", "call page.snapshot before passing uid", false)
	}
	if snap.TargetID != att.Target.TargetID {
		return nil, tools.Fail(tools.CategoryNotFound, "snapshot_target_mismatch",
			fmt.Sprintf("snapshot was taken on target %s; current target is %s", snap.TargetID, att.Target.TargetID), false)
	}
	node, ok := snap.Nodes[uid]
	if !ok {
		return nil, tools.Fail(tools.CategoryNotFound, "uid_not_found",
			fmt.Sprintf("uid %s not found in current snapshot", uid), false)
	}
	if err := env.EnsureEnabled(ctx, att.SessionID, "DOM"); err != nil {
		return nil, tools.ClassifyCDPErr("enable_dom_failed", err)
	}
	rawBox, err := att.Conn.Send(ctx, att.SessionID, "DOM.getBoxModel", map[string]any{
		"backendNodeId": node.BackendNodeID,
	})
	if err != nil {
		return nil, tools.ClassifyCDPErr("get_box_model_failed", err)
	}
	var box struct {
		Model struct {
			Content []float64 `json:"content"`
			Width   float64   `json:"width"`
			Height  float64   `json:"height"`
		} `json:"model"`
	}
	if err := json.Unmarshal(rawBox, &box); err != nil {
		return nil, tools.Fail(tools.CategoryProtocol, "box_decode", err.Error(), false)
	}
	if len(box.Model.Content) < 8 {
		return nil, tools.Fail(tools.CategoryProtocol, "box_quad_invalid", "content quad too short", false)
	}
	x := box.Model.Content[0]
	y := box.Model.Content[1]
	return map[string]any{
		"x":      x,
		"y":      y,
		"width":  box.Model.Width,
		"height": box.Model.Height,
		"scale":  1,
	}, tools.Result{}
}
