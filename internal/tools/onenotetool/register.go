package onenotetool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds all onenote.* tools to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(ReadNotebooks())
	r.MustRegister(ReadSections())
	r.MustRegister(ReadPages())
	r.MustRegister(ReadPage())
	r.MustRegister(AddPage())
	r.MustRegister(RunScript())
}
