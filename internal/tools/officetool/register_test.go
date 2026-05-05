package officetool

import (
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/officejs"
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRegisterCompilesSchemas(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)
	if got := len(r.List()); got != 1 {
		t.Fatalf("expected 1 office.* tool registered (embed), got %d", got)
	}
}

// TestEmbedDependenciesPresent guards the Office.js payloads office.embed
// invokes via env.Attach + executor.Run (excel.readRange and
// powerpoint.insertTextTable). No matching JS file = runtime failure. The
// embed tool itself does not have a payload of its own name; it stitches
// these two together in Go.
func TestEmbedDependenciesPresent(t *testing.T) {
	if err := officejs.Preload(); err != nil {
		t.Fatalf("preload payloads: %v", err)
	}
	available := map[string]bool{}
	for _, n := range officejs.Names() {
		available[n] = true
	}
	for _, dep := range []string{"excel.readRange", "powerpoint.insertTextTable"} {
		if !available[dep] {
			t.Errorf("office.embed depends on payload %q but no JS file found", dep)
		}
	}
}
