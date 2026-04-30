package cdptool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds all cdp.* tools to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(Evaluate())
	r.MustRegister(GetTargets())
	r.MustRegister(SelectTarget())
}
