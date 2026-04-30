package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func minimalTool(name string) Tool {
	return Tool{
		Name:   name,
		Schema: json.RawMessage(`{"type":"object"}`),
		Run: func(_ context.Context, _ json.RawMessage, _ *RunEnv) Result {
			return OK(nil)
		},
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(minimalTool("a.b")); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := r.Register(minimalTool("a.b")); err == nil {
		t.Fatal("expected duplicate-registration error")
	}
}

func TestRegistry_RegisterEmptyName(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Tool{Run: minimalTool("x").Run}); err == nil {
		t.Fatal("expected empty-name error")
	}
}

func TestRegistry_RegisterNilRun(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Tool{Name: "x"}); err == nil {
		t.Fatal("expected nil-Run error")
	}
}

func TestRegistry_RegisterBadSchema(t *testing.T) {
	r := NewRegistry()
	bad := Tool{
		Name:   "bad",
		Schema: json.RawMessage(`{"type":"not-a-type"}`),
		Run:    minimalTool("bad").Run,
	}
	if err := r.Register(bad); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("expected schema error, got %v", err)
	}
}

func TestRegistry_ListSorted(t *testing.T) {
	r := NewRegistry()
	for _, n := range []string{"z.x", "a.x", "m.x"} {
		r.MustRegister(minimalTool(n))
	}
	got := r.List()
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Name != "a.x" || got[1].Name != "m.x" || got[2].Name != "z.x" {
		t.Errorf("not sorted: %+v", got)
	}
}
