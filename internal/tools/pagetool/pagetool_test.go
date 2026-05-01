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
