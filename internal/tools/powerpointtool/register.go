package powerpointtool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds the powerpoint.* tool surface to the registry.
//
// Phase 0 of PLAN-workflow-surface narrowed this to the runScript escape
// hatch only. Primitive constructors stay in the package as reusable
// building blocks for Phase A workflow tools.
func Register(r *tools.Registry) {
	r.MustRegister(RunScript())
}
