package powerpointtool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds all powerpoint.* tools to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(ReadPresentation())
	r.MustRegister(ReadSlides())
	r.MustRegister(ReadSlide())
	r.MustRegister(AddSlide())
	r.MustRegister(ReadSelection())
	r.MustRegister(RunScript())
}
