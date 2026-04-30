package browsertool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds all browser.* tools to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(Navigate())
}
