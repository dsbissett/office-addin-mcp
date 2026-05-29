// Package inspecttool registers the page.* read-only tools (snapshot,
// screenshot, waitFor, evaluate). Snapshot installs a UID → backendNodeId
// table on the session that page.click / page.fill / page.hover use to
// resolve targets without exposing raw nodeIds to the agent.
package inspecttool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dsbissett/office-addin-mcp/internal/session"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

const snapshotSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "page.snapshot parameters",
  "type": "object",
  "properties": {
    "targetId":   {"type": "string", "description": "Exact target id; mutually exclusive with urlPattern/surface."},
    "urlPattern": {"type": "string", "description": "Substring of the target URL."},
    "surface":    {"type": "string", "enum": ["taskpane", "content", "dialog", "cf-runtime"], "description": "Manifest-classified surface."},
    "maxChars":   {"type": "integer", "minimum": 100, "description": "Hard cap on the returned text snapshot. Default 5000."}
  },
  "additionalProperties": false
}`

type snapshotParams struct {
	TargetID   string `json:"targetId,omitempty"`
	URLPattern string `json:"urlPattern,omitempty"`
	Surface    string `json:"surface,omitempty"`
	MaxChars   int    `json:"maxChars,omitempty"`
}

const defaultSnapshotMaxChars = 5000

// Snapshot returns the page.snapshot tool. It walks the active page's
// accessibility tree, assigns a stable uid to each interesting node, and
// caches uid → backendNodeId on the session for the lifetime of the snapshot.
// The agent-visible payload is a compact text outline (`[uid-3] button "OK"`)
// agents can quote in subsequent page.click(uid) calls.
func Snapshot() tools.Tool {
	return tools.Tool{
		Name:        "page.snapshot",
		Description: "Capture an accessibility-tree snapshot of the active page and return a UID-tagged text outline. UIDs are usable in page.click / page.fill / page.hover until the next snapshot.",
		Schema:      json.RawMessage(snapshotSchema),
		Annotations: &tools.Annotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: tools.BoolPtr(false)},
		Run:         runSnapshot,
	}
}

func runSnapshot(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p snapshotParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	maxChars := snapshotMaxChars(p)

	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}

	tree, fail := fetchAXTree(ctx, env, att)
	if fail != nil {
		return *fail
	}

	nodes, lines := walkAXTree(tree.Nodes)

	if env.SetSnapshot != nil {
		env.SetSnapshot(&session.Snapshot{
			TargetID:     att.Target.TargetID,
			CDPSessionID: att.SessionID,
			Nodes:        nodes,
		})
	}

	text, truncated := clipSnapshotText(strings.Join(lines, "\n"), maxChars)
	return tools.OKWithSummary(
		fmt.Sprintf("Captured snapshot of %d node(s)%s.", len(nodes), truncatedSuffix(truncated)),
		struct {
			TargetID  string `json:"targetId"`
			URL       string `json:"url"`
			Title     string `json:"title,omitempty"`
			NodeCount int    `json:"nodeCount"`
			Snapshot  string `json:"snapshot"`
			Truncated bool   `json:"truncated,omitempty"`
		}{
			TargetID:  att.Target.TargetID,
			URL:       att.Target.URL,
			Title:     att.Target.Title,
			NodeCount: len(nodes),
			Snapshot:  text,
			Truncated: truncated,
		},
	)
}

// snapshotMaxChars resolves the snapshot text cap, defaulting when unset.
func snapshotMaxChars(p snapshotParams) int {
	if p.MaxChars <= 0 {
		return defaultSnapshotMaxChars
	}
	return p.MaxChars
}

// fetchAXTree enables Accessibility on the target, fetches the full AX tree,
// and decodes it. It returns the decoded tree or a non-nil failure Result.
func fetchAXTree(ctx context.Context, env *tools.RunEnv, att *tools.AttachedTarget) (axTreeResult, *tools.Result) {
	if err := env.EnsureEnabled(ctx, att.SessionID, "Accessibility"); err != nil {
		return axTreeResult{}, ptrSnapshotResult(tools.ClassifyCDPErr("enable_accessibility_failed", err))
	}
	rawTree, err := att.Conn.Send(ctx, att.SessionID, "Accessibility.getFullAXTree", map[string]any{})
	if err != nil {
		return axTreeResult{}, ptrSnapshotResult(tools.ClassifyCDPErr("ax_tree_failed", err))
	}
	var tree axTreeResult
	if err := json.Unmarshal(rawTree, &tree); err != nil {
		return axTreeResult{}, ptrSnapshotResult(tools.Fail(tools.CategoryProtocol, "ax_tree_decode", err.Error(), false))
	}
	return tree, nil
}

// clipSnapshotText caps text to maxChars, reporting whether it was truncated.
func clipSnapshotText(text string, maxChars int) (string, bool) {
	if len(text) > maxChars {
		return text[:maxChars], true
	}
	return text, false
}

// truncatedSuffix returns the summary suffix for a truncated snapshot.
func truncatedSuffix(truncated bool) string {
	if truncated {
		return " (truncated)"
	}
	return ""
}

// ptrSnapshotResult boxes a Result for the (value, *Result) early-exit
// convention.
func ptrSnapshotResult(r tools.Result) *tools.Result { return &r }

// axTreeResult is the subset of Accessibility.getFullAXTree we consume.
type axTreeResult struct {
	Nodes []axNode `json:"nodes"`
}

type axNode struct {
	NodeID        string         `json:"nodeId"`
	BackendDOMID  int            `json:"backendDOMNodeId"`
	Role          axProp         `json:"role"`
	Name          axProp         `json:"name"`
	Value         axProp         `json:"value"`
	Description   axProp         `json:"description"`
	Ignored       bool           `json:"ignored"`
	IgnoredReason []axIgnoreItem `json:"ignoredReasons,omitempty"`
	ChildIDs      []string       `json:"childIds,omitempty"`
	ParentID      string         `json:"parentId,omitempty"`
}

type axProp struct {
	Type  string          `json:"type,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

type axIgnoreItem struct {
	Name string `json:"name"`
}

func (p axProp) string() string {
	if len(p.Value) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(p.Value, &s); err == nil {
		return s
	}
	return strings.Trim(string(p.Value), `"`)
}

// walkAXTree builds the UID → SnapshotNode map and a flat outline of lines,
// indented by tree depth. Ignored or boring (no role) nodes are skipped from
// the user-visible outline but still walked through for their children.
func walkAXTree(all []axNode) (map[string]session.SnapshotNode, []string) {
	if len(all) == 0 {
		return map[string]session.SnapshotNode{}, nil
	}
	byID := indexAXNodes(all)
	w := &axWalker{byID: byID, nodes: map[string]session.SnapshotNode{}}
	for _, r := range axRoots(all, byID) {
		w.walk(r, 0)
	}
	return w.nodes, w.lines
}

// indexAXNodes maps every node by its NodeID for parent/child lookups.
func indexAXNodes(all []axNode) map[string]*axNode {
	byID := make(map[string]*axNode, len(all))
	for i := range all {
		byID[all[i].NodeID] = &all[i]
	}
	return byID
}

// axRoots returns the tree root(s): nodes whose parent is empty or absent.
// Falls back to the first node when no root is identifiable.
func axRoots(all []axNode, byID map[string]*axNode) []*axNode {
	var roots []*axNode
	for i := range all {
		n := &all[i]
		if isAXRoot(n, byID) {
			roots = append(roots, n)
		}
	}
	if len(roots) == 0 {
		return []*axNode{&all[0]}
	}
	return roots
}

// isAXRoot reports whether a node has no in-set parent.
func isAXRoot(n *axNode, byID map[string]*axNode) bool {
	if n.ParentID == "" {
		return true
	}
	_, ok := byID[n.ParentID]
	return !ok
}

// axWalker carries the mutable state threaded through the recursive AX walk.
type axWalker struct {
	byID       map[string]*axNode
	nodes      map[string]session.SnapshotNode
	lines      []string
	uidCounter int
}

func (w *axWalker) walk(n *axNode, depth int) {
	role := n.Role.string()
	visible := axNodeVisible(n, role)
	if visible {
		w.uidCounter++
		w.record(n, depth, role)
	}
	w.walkChildren(n, axChildDepth(depth, visible))
}

// record emits the UID-tagged node and outline line when it has a DOM backing.
func (w *axWalker) record(n *axNode, depth int, role string) {
	if n.BackendDOMID <= 0 {
		return
	}
	uid := fmt.Sprintf("uid-%d", w.uidCounter)
	name := n.Name.string()
	w.nodes[uid] = session.SnapshotNode{
		UID:           uid,
		BackendNodeID: n.BackendDOMID,
		Role:          role,
		Name:          name,
	}
	w.lines = append(w.lines, formatNode(uid, depth, role, name, n.Value.string()))
}

// walkChildren recurses into the in-set children of n at the given depth.
func (w *axWalker) walkChildren(n *axNode, depth int) {
	for _, cid := range n.ChildIDs {
		if c, ok := w.byID[cid]; ok {
			w.walk(c, depth)
		}
	}
}

// axNodeVisible reports whether a node should appear in the user-visible
// outline (not ignored and carrying a meaningful role).
func axNodeVisible(n *axNode, role string) bool {
	return !n.Ignored && role != "" && role != "none" && role != "presentation"
}

// axChildDepth returns the indent depth for a node's children, descending one
// level only past visible nodes.
func axChildDepth(depth int, visible bool) int {
	if visible {
		return depth + 1
	}
	return depth
}

func formatNode(uid string, depth int, role, name, value string) string {
	indent := strings.Repeat("  ", depth)
	parts := []string{indent, "[", uid, "] ", role}
	if name != "" {
		parts = append(parts, " ", quote(name))
	}
	if value != "" {
		parts = append(parts, " value=", quote(value))
	}
	return strings.Join(parts, "")
}

func quote(s string) string {
	if len(s) > 80 {
		s = s[:77] + "..."
	}
	b, _ := json.Marshal(s)
	return string(b)
}
