package inspecttool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/cdp"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// axTreeReply builds an Accessibility.getFullAXTree CDP reply from a node set.
func axTreeReply(nodes []axNode) map[string]any {
	return map[string]any{"nodes": nodes}
}

func sampleTree() []axNode {
	return []axNode{
		{NodeID: "1", Role: prop("WebArea"), Name: prop("Doc"), BackendDOMID: 100, ChildIDs: []string{"2", "3"}},
		{NodeID: "2", ParentID: "1", Role: prop("button"), Name: prop("OK"), BackendDOMID: 200},
		{NodeID: "3", ParentID: "1", Role: prop("textbox"), Name: prop("Email"), Value: prop("a@b.com"), BackendDOMID: 300},
	}
}

func TestRunSnapshot_HappyPath(t *testing.T) {
	sess := newSession()
	env, _ := fakeEnv(t, fakeEnvOpts{
		sess:   sess,
		target: cdp.TargetInfo{TargetID: "T-1", URL: "https://localhost:3000/taskpane.html", Title: "Task Pane"},
		resp: func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
			if method == "Accessibility.getFullAXTree" {
				return axTreeReply(sampleTree()), nil
			}
			return map[string]any{}, nil
		},
	})
	res := runSnapshot(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out, ok := res.Data.(struct {
		TargetID  string `json:"targetId"`
		URL       string `json:"url"`
		Title     string `json:"title,omitempty"`
		NodeCount int    `json:"nodeCount"`
		Snapshot  string `json:"snapshot"`
		Truncated bool   `json:"truncated,omitempty"`
	})
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if out.NodeCount != 3 {
		t.Errorf("nodeCount=%d, want 3", out.NodeCount)
	}
	if out.TargetID != "T-1" || out.URL != "https://localhost:3000/taskpane.html" {
		t.Errorf("target metadata lost: %+v", out)
	}
	if !strings.Contains(out.Snapshot, "button") || !strings.Contains(out.Snapshot, "value=") {
		t.Errorf("snapshot text missing expected content: %q", out.Snapshot)
	}
	// SetSnapshot should have stored a snapshot keyed to the target.
	snap := sess.Snapshot()
	if snap == nil || snap.TargetID != "T-1" || len(snap.Nodes) != 3 {
		t.Errorf("snapshot not installed correctly: %+v", snap)
	}
}

func TestRunSnapshot_Truncation(t *testing.T) {
	// Build a large tree so the joined outline exceeds maxChars.
	var nodes []axNode
	nodes = append(nodes, axNode{NodeID: "root", Role: prop("WebArea"), BackendDOMID: 1})
	var childIDs []string
	for i := 0; i < 50; i++ {
		id := "n" + string(rune('A'+i%26)) + string(rune('0'+i/26))
		childIDs = append(childIDs, id)
		nodes = append(nodes, axNode{
			NodeID: id, ParentID: "root", Role: prop("button"),
			Name: prop(strings.Repeat("x", 40)), BackendDOMID: 100 + i,
		})
	}
	nodes[0].ChildIDs = childIDs
	env, _ := fakeEnv(t, fakeEnvOpts{
		target: cdp.TargetInfo{TargetID: "T-1"},
		resp: func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
			if method == "Accessibility.getFullAXTree" {
				return axTreeReply(nodes), nil
			}
			return map[string]any{}, nil
		},
	})
	res := runSnapshot(context.Background(), json.RawMessage(`{"maxChars":100}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	out := res.Data.(struct {
		TargetID  string `json:"targetId"`
		URL       string `json:"url"`
		Title     string `json:"title,omitempty"`
		NodeCount int    `json:"nodeCount"`
		Snapshot  string `json:"snapshot"`
		Truncated bool   `json:"truncated,omitempty"`
	})
	if !out.Truncated {
		t.Errorf("expected truncated=true")
	}
	if len(out.Snapshot) != 100 {
		t.Errorf("snapshot len=%d, want 100", len(out.Snapshot))
	}
	if !strings.Contains(res.Summary, "truncated") {
		t.Errorf("summary should mention truncation: %q", res.Summary)
	}
}

func TestRunSnapshot_NilSetSnapshot(t *testing.T) {
	// SetSnapshot nil → the run should still succeed without installing.
	env := &tools.RunEnv{
		Diag: &tools.Diagnostics{},
		Attach: func(context.Context, tools.TargetSelector) (*tools.AttachedTarget, error) {
			srv := cdptestServer(t, func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
				if method == "Accessibility.getFullAXTree" {
					return axTreeReply(sampleTree()), nil
				}
				return map[string]any{}, nil
			})
			return &tools.AttachedTarget{Conn: srv, SessionID: "cdp-1", Target: cdp.TargetInfo{TargetID: "T-1"}}, nil
		},
		EnsureEnabled: func(context.Context, string, string) error { return nil },
		// SetSnapshot intentionally nil.
	}
	res := runSnapshot(context.Background(), json.RawMessage(`{}`), env)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
}

func TestRunSnapshot_EnableAccessibilityFailed(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		enableErr: errors.New("a11y enable failed"),
		resp: func(string, json.RawMessage) (any, *cdp.RemoteError) {
			return axTreeReply(sampleTree()), nil
		},
	})
	res := runSnapshot(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "enable_accessibility_failed" {
		t.Fatalf("want enable_accessibility_failed, got %+v", res.Err)
	}
}

func TestRunSnapshot_AXTreeSendError(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
			if method == "Accessibility.getFullAXTree" {
				return nil, &cdp.RemoteError{Code: -32000, Message: "no a11y"}
			}
			return map[string]any{}, nil
		},
	})
	res := runSnapshot(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "ax_tree_failed" {
		t.Fatalf("want ax_tree_failed, got %+v", res.Err)
	}
}

func TestRunSnapshot_AXTreeDecodeError(t *testing.T) {
	env, _ := fakeEnv(t, fakeEnvOpts{
		resp: func(method string, _ json.RawMessage) (any, *cdp.RemoteError) {
			if method == "Accessibility.getFullAXTree" {
				// nodes is a string, not an array → ax_tree_decode.
				return map[string]any{"nodes": "garbage"}, nil
			}
			return map[string]any{}, nil
		},
	})
	res := runSnapshot(context.Background(), json.RawMessage(`{}`), env)
	if res.Err == nil || res.Err.Code != "ax_tree_decode" {
		t.Fatalf("want ax_tree_decode, got %+v", res.Err)
	}
}

func TestRunSnapshot_AttachFailure(t *testing.T) {
	res := runSnapshot(context.Background(), json.RawMessage(`{}`), errEnv())
	if res.Err == nil || res.Err.Code != "attach_failed" {
		t.Fatalf("want attach_failed, got %+v", res.Err)
	}
}

func TestRunSnapshot_BadParams(t *testing.T) {
	res := runSnapshot(context.Background(), json.RawMessage(`{"maxChars":"big"}`), errEnv())
	if res.Err == nil || res.Err.Code != "param_decode" {
		t.Fatalf("want param_decode, got %+v", res.Err)
	}
}

func TestSnapshot_ToolMetadata(t *testing.T) {
	tool := Snapshot()
	if tool.Name != "page.snapshot" || tool.Run == nil {
		t.Errorf("unexpected tool metadata: %+v", tool)
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Schema, &schema); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
}

// --- walkAXTree pure-function coverage beyond the existing test ---

func TestWalkAXTree_Empty(t *testing.T) {
	nodes, lines := walkAXTree(nil)
	if len(nodes) != 0 || lines != nil {
		t.Errorf("empty tree should yield no nodes/lines, got %d nodes, %v lines", len(nodes), lines)
	}
}

func TestWalkAXTree_IgnoredAndPresentation(t *testing.T) {
	tree := []axNode{
		{NodeID: "1", Role: prop("WebArea"), BackendDOMID: 1, ChildIDs: []string{"2", "3", "4"}},
		{NodeID: "2", ParentID: "1", Role: prop("button"), Name: prop("Visible"), BackendDOMID: 2},
		{NodeID: "3", ParentID: "1", Role: prop("button"), Ignored: true, BackendDOMID: 3},
		{NodeID: "4", ParentID: "1", Role: prop("presentation"), BackendDOMID: 4},
	}
	nodes, _ := walkAXTree(tree)
	// WebArea + Visible button only (ignored + presentation skipped).
	if len(nodes) != 2 {
		t.Errorf("expected 2 visible nodes, got %d", len(nodes))
	}
}

func TestWalkAXTree_ZeroBackendIDNotMapped(t *testing.T) {
	// A visible node with BackendDOMID == 0 is skipped from the uid map but
	// its children are still walked.
	tree := []axNode{
		{NodeID: "1", Role: prop("WebArea"), BackendDOMID: 0, ChildIDs: []string{"2"}},
		{NodeID: "2", ParentID: "1", Role: prop("button"), Name: prop("Deep"), BackendDOMID: 9},
	}
	nodes, _ := walkAXTree(tree)
	if len(nodes) != 1 {
		t.Errorf("expected only the backed child node, got %d", len(nodes))
	}
	if _, ok := nodes["uid-2"]; !ok {
		t.Errorf("expected the child to keep its uid-2 slot; nodes=%v", nodes)
	}
}

func TestWalkAXTree_OrphanParentBecomesRoot(t *testing.T) {
	// Node references a parent that isn't in the set → treated as a root.
	tree := []axNode{
		{NodeID: "x", ParentID: "missing", Role: prop("button"), Name: prop("Orphan"), BackendDOMID: 5},
	}
	nodes, lines := walkAXTree(tree)
	if len(nodes) != 1 || len(lines) != 1 {
		t.Errorf("orphan should still be walked as a root; nodes=%d lines=%d", len(nodes), len(lines))
	}
}

func TestAXProp_String(t *testing.T) {
	// Empty value → "".
	if got := (axProp{}).string(); got != "" {
		t.Errorf("empty axProp string=%q", got)
	}
	// Non-string JSON value falls back to trimmed raw.
	p := axProp{Value: json.RawMessage(`123`)}
	if got := p.string(); got != "123" {
		t.Errorf("numeric axProp string=%q, want 123", got)
	}
	// String value unmarshals.
	if got := prop("hello").string(); got != "hello" {
		t.Errorf("string axProp=%q, want hello", got)
	}
}

func TestQuote_ShortAndLong(t *testing.T) {
	if got := quote("hi"); got != `"hi"` {
		t.Errorf("quote(short)=%q", got)
	}
	long := strings.Repeat("y", 200)
	got := quote(long)
	if !strings.Contains(got, "...") {
		t.Errorf("quote(long) should truncate: %q", got)
	}
}

func TestFormatNode_WithValueAndIndent(t *testing.T) {
	out := formatNode("uid-3", 2, "textbox", "Email", "a@b.com")
	if !strings.HasPrefix(out, "    ") { // depth 2 → 4 spaces
		t.Errorf("indent wrong: %q", out)
	}
	if !strings.Contains(out, "[uid-3]") || !strings.Contains(out, "value=") {
		t.Errorf("formatted node missing parts: %q", out)
	}
}
