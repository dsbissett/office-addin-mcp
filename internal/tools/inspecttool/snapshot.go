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
		Run:         runSnapshot,
	}
}

func runSnapshot(ctx context.Context, raw json.RawMessage, env *tools.RunEnv) tools.Result {
	var p snapshotParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return tools.Fail(tools.CategoryValidation, "param_decode", err.Error(), false)
	}
	maxChars := p.MaxChars
	if maxChars <= 0 {
		maxChars = defaultSnapshotMaxChars
	}

	att, err := env.Attach(ctx, makeSelector(p.TargetID, p.URLPattern, p.Surface))
	if err != nil {
		return tools.Fail(tools.CategoryNotFound, "attach_failed", err.Error(), false)
	}

	if err := env.EnsureEnabled(ctx, att.SessionID, "Accessibility"); err != nil {
		return tools.ClassifyCDPErr("enable_accessibility_failed", err)
	}

	rawTree, err := att.Conn.Send(ctx, att.SessionID, "Accessibility.getFullAXTree", map[string]any{})
	if err != nil {
		return tools.ClassifyCDPErr("ax_tree_failed", err)
	}
	var tree axTreeResult
	if err := json.Unmarshal(rawTree, &tree); err != nil {
		return tools.Fail(tools.CategoryProtocol, "ax_tree_decode", err.Error(), false)
	}

	nodes, lines := walkAXTree(tree.Nodes)

	if env.SetSnapshot != nil {
		env.SetSnapshot(&session.Snapshot{
			TargetID:     att.Target.TargetID,
			CDPSessionID: att.SessionID,
			Nodes:        nodes,
		})
	}

	text := strings.Join(lines, "\n")
	truncated := false
	if len(text) > maxChars {
		text = text[:maxChars]
		truncated = true
	}

	return tools.OK(struct {
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
	})
}

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
	byID := make(map[string]*axNode, len(all))
	for i := range all {
		byID[all[i].NodeID] = &all[i]
	}

	// Find root(s): nodes whose parent is empty or not present.
	var roots []*axNode
	for i := range all {
		n := &all[i]
		if n.ParentID == "" {
			roots = append(roots, n)
			continue
		}
		if _, ok := byID[n.ParentID]; !ok {
			roots = append(roots, n)
		}
	}
	if len(roots) == 0 && len(all) > 0 {
		roots = []*axNode{&all[0]}
	}

	nodes := map[string]session.SnapshotNode{}
	var lines []string
	uidCounter := 0
	var walk func(n *axNode, depth int)
	walk = func(n *axNode, depth int) {
		role := n.Role.string()
		name := n.Name.string()
		visible := !n.Ignored && role != "" && role != "none" && role != "presentation"
		if visible {
			uidCounter++
			uid := fmt.Sprintf("uid-%d", uidCounter)
			if n.BackendDOMID > 0 {
				nodes[uid] = session.SnapshotNode{
					UID:           uid,
					BackendNodeID: n.BackendDOMID,
					Role:          role,
					Name:          name,
				}
				lines = append(lines, formatNode(uid, depth, role, name, n.Value.string()))
			}
		}
		nextDepth := depth
		if visible {
			nextDepth++
		}
		for _, cid := range n.ChildIDs {
			if c, ok := byID[cid]; ok {
				walk(c, nextDepth)
			}
		}
	}
	for _, r := range roots {
		walk(r, 0)
	}
	return nodes, lines
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
