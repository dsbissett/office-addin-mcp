package officetool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds the cross-host office.* workflow tools to the registry.
//
// Phase A introduces this surface alongside the per-host workflow tools.
// Cross-host tools sequence calls against multiple CDP targets in one Go
// dispatch, returning a single envelope.
func Register(r *tools.Registry) {
	r.MustRegister(Embed())
}
