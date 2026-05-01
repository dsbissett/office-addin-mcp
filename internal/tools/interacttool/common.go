// Package interacttool registers UID-driven interaction tools (page.click,
// page.fill, page.hover, page.typeText, page.pressKey). UIDs come from a
// preceding page.snapshot; the resolver consults the session's snapshot
// cache to translate them into backendNodeIds for CDP DOM/Input commands.
package interacttool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dsbissett/office-addin-mcp/internal/addin"
	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// nodeCenter resolves a UID to a (x, y) center point in CSS pixels via
// DOM.getBoxModel. The session must have a snapshot whose target matches the
// active attached target.
func nodeCenter(ctx context.Context, env *tools.RunEnv, att *tools.AttachedTarget, uid string) (float64, float64, *session.SnapshotNode, tools.Result) {
	node, res := lookupNode(env, att, uid)
	if res.Err != nil {
		return 0, 0, nil, res
	}
	if err := env.EnsureEnabled(ctx, att.SessionID, "DOM"); err != nil {
		return 0, 0, nil, tools.ClassifyCDPErr("enable_dom_failed", err)
	}
	rawBox, err := att.Conn.Send(ctx, att.SessionID, "DOM.getBoxModel", map[string]any{
		"backendNodeId": node.BackendNodeID,
	})
	if err != nil {
		return 0, 0, nil, tools.ClassifyCDPErr("get_box_model_failed", err)
	}
	var box struct {
		Model struct {
			Content []float64 `json:"content"`
		} `json:"model"`
	}
	if err := json.Unmarshal(rawBox, &box); err != nil {
		return 0, 0, nil, tools.Fail(tools.CategoryProtocol, "box_decode", err.Error(), false)
	}
	if len(box.Model.Content) < 8 {
		return 0, 0, nil, tools.Fail(tools.CategoryProtocol, "box_quad_invalid", "content quad too short", false)
	}
	x := (box.Model.Content[0] + box.Model.Content[4]) / 2
	y := (box.Model.Content[1] + box.Model.Content[5]) / 2
	return x, y, node, tools.Result{}
}

func lookupNode(env *tools.RunEnv, att *tools.AttachedTarget, uid string) (*session.SnapshotNode, tools.Result) {
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
	return &node, tools.Result{}
}

func makeSelector(targetID, urlPattern, surface string) tools.TargetSelector {
	return tools.TargetSelector{
		TargetID:   targetID,
		URLPattern: urlPattern,
		Surface:    addin.SurfaceType(surface),
	}
}

type selectorCommon struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
	Surface    string `json:"surface,omitempty"`
}

func (s selectorCommon) selector() tools.TargetSelector {
	return makeSelector(s.TargetID, s.URLPattern, s.Surface)
}
