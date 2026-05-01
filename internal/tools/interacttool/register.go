package interacttool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds the page.click / page.fill / page.hover / page.typeText /
// page.pressKey tools to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(Click())
	r.MustRegister(Fill())
	r.MustRegister(Hover())
	r.MustRegister(TypeText())
	r.MustRegister(PressKey())
}
