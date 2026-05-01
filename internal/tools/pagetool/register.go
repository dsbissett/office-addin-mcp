package pagetool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds pages.list / pages.select / pages.close / pages.handleDialog
// and page.navigate to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(List())
	r.MustRegister(Select())
	r.MustRegister(Close())
	r.MustRegister(HandleDialog())
	r.MustRegister(Navigate())
}
