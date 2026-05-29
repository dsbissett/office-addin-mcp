package pagetool

import (
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRegister_AllTools(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)
	for _, name := range []string{"pages.list", "pages.select", "pages.close", "pages.handleDialog", "page.navigate"} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("tool %s not registered", name)
		}
	}
}

func TestMakeSelector_PassesThrough(t *testing.T) {
	sel := makeSelector("T1", "localhost", "taskpane")
	if sel.TargetID != "T1" || sel.URLPattern != "localhost" || string(sel.Surface) != "taskpane" {
		t.Errorf("unexpected selector: %+v", sel)
	}
}

// derefBool reports the value behind a *bool, defaulting nil to want so a
// "must be set" assertion can distinguish nil from the wrong value.
func derefBool(p *bool) (val bool, set bool) {
	if p == nil {
		return false, false
	}
	return *p, true
}

func TestAnnotations_ReadOnlyTool(t *testing.T) {
	a := List().Annotations
	if a == nil {
		t.Fatal("pages.list must declare annotations")
	}
	if !a.ReadOnlyHint {
		t.Error("pages.list ReadOnlyHint should be true")
	}
	if !a.IdempotentHint {
		t.Error("pages.list IdempotentHint should be true")
	}
	if d, set := derefBool(a.DestructiveHint); !set || d {
		t.Errorf("pages.list DestructiveHint should be false, got set=%v val=%v", set, d)
	}
}

func TestAnnotations_DestructiveTools(t *testing.T) {
	for name, tool := range map[string]func() tools.Tool{
		"pages.close":        Close,
		"pages.handleDialog": HandleDialog,
		"page.navigate":      Navigate,
	} {
		a := tool().Annotations
		if a == nil {
			t.Fatalf("%s must declare annotations", name)
		}
		if a.ReadOnlyHint {
			t.Errorf("%s ReadOnlyHint must not be true", name)
		}
		if d, set := derefBool(a.DestructiveHint); !set || !d {
			t.Errorf("%s DestructiveHint should be true, got set=%v val=%v", name, set, d)
		}
	}
}

func TestAnnotations_AdditiveMutation(t *testing.T) {
	a := Select().Annotations
	if a == nil {
		t.Fatal("pages.select must declare annotations")
	}
	if a.ReadOnlyHint {
		t.Error("pages.select ReadOnlyHint must not be true (it mutates the sticky default)")
	}
	if d, set := derefBool(a.DestructiveHint); !set || d {
		t.Errorf("pages.select DestructiveHint should be false, got set=%v val=%v", set, d)
	}
}

func TestAnnotations_NavigateOpenWorld(t *testing.T) {
	a := Navigate().Annotations
	if w, set := derefBool(a.OpenWorldHint); !set || !w {
		t.Errorf("page.navigate OpenWorldHint should be true, got set=%v val=%v", set, w)
	}
}

func TestAnnotations_AllToolsHaveAnnotations(t *testing.T) {
	for name, tool := range map[string]func() tools.Tool{
		"pages.list":         List,
		"pages.select":       Select,
		"pages.close":        Close,
		"pages.handleDialog": HandleDialog,
		"page.navigate":      Navigate,
	} {
		if tool().Annotations == nil {
			t.Errorf("%s is missing annotations", name)
		}
	}
}
