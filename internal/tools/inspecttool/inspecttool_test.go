package inspecttool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRegister_AllTools(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)
	for _, name := range []string{
		"page.snapshot",
		"page.screenshot",
		"page.waitFor",
		"page.evaluate",
		"page.consoleLog",
		"page.networkLog",
		"page.networkBody",
	} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("tool %s not registered", name)
		}
	}
}

func TestAnnotations_ReadOnlyTools(t *testing.T) {
	cases := map[string]tools.Tool{
		"page.snapshot":    Snapshot(),
		"page.screenshot":  Screenshot(),
		"page.waitFor":     WaitFor(),
		"page.consoleLog":  ConsoleLog(),
		"page.networkLog":  NetworkLog(),
		"page.networkBody": NetworkBody(),
	}
	for name, tool := range cases {
		a := tool.Annotations
		if a == nil {
			t.Fatalf("%s: missing annotations", name)
		}
		if !a.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint = false, want true", name)
		}
		if !a.IdempotentHint {
			t.Errorf("%s: IdempotentHint = false, want true", name)
		}
		if a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("%s: DestructiveHint = %v, want non-nil false", name, a.DestructiveHint)
		}
	}
}

func TestAnnotations_EvaluateIsDestructiveOpenWorld(t *testing.T) {
	a := Evaluate().Annotations
	if a == nil {
		t.Fatal("page.evaluate: missing annotations")
	}
	if a.ReadOnlyHint {
		t.Error("page.evaluate: ReadOnlyHint = true, want false")
	}
	if a.DestructiveHint == nil || !*a.DestructiveHint {
		t.Errorf("page.evaluate: DestructiveHint = %v, want non-nil true", a.DestructiveHint)
	}
	if a.OpenWorldHint == nil || !*a.OpenWorldHint {
		t.Errorf("page.evaluate: OpenWorldHint = %v, want non-nil true", a.OpenWorldHint)
	}
}

func TestWalkAXTree_AssignsUIDsAndSkipsIgnored(t *testing.T) {
	tree := []axNode{
		{NodeID: "1", Role: prop("WebArea"), Name: prop("Doc"), BackendDOMID: 100, ChildIDs: []string{"2", "3"}},
		{NodeID: "2", ParentID: "1", Role: prop("button"), Name: prop("OK"), BackendDOMID: 200},
		{NodeID: "3", ParentID: "1", Role: prop("none"), BackendDOMID: 300, ChildIDs: []string{"4"}},
		{NodeID: "4", ParentID: "3", Role: prop("textbox"), Name: prop("Email"), BackendDOMID: 400},
	}
	nodes, lines := walkAXTree(tree)
	if len(nodes) != 3 {
		t.Fatalf("expected 3 visible nodes, got %d", len(nodes))
	}
	want := []string{"WebArea", "button", "textbox"}
	for i, ln := range lines {
		if !strings.Contains(ln, want[i]) {
			t.Errorf("line[%d]=%q does not contain %q", i, ln, want[i])
		}
	}
	for uid, sn := range nodes {
		if !strings.HasPrefix(uid, "uid-") {
			t.Errorf("uid %q does not start with uid-", uid)
		}
		if sn.BackendNodeID == 0 {
			t.Errorf("snapshot node %q has zero backend id", uid)
		}
	}
}

func TestParseShortcutFormatsHelper(t *testing.T) {
	// formatNode/quote contract: long names are truncated to ~80 chars.
	long := strings.Repeat("x", 200)
	out := formatNode("uid-1", 0, "textbox", long, "")
	if !strings.Contains(out, "...") {
		t.Errorf("expected truncation marker in %q", out)
	}
}

func prop(s string) axProp {
	b, _ := json.Marshal(s)
	return axProp{Type: "role", Value: b}
}
