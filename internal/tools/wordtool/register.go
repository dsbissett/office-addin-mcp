package wordtool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds the word.* tool surface to the registry.
//
// Phase 0 of PLAN-workflow-surface narrowed this to the runScript escape
// hatch. Phase A adds workflow-shaped tools (applyEdits). Primitive
// constructors stay in the package as reusable building blocks.
func Register(r *tools.Registry) {
	r.MustRegister(RunScript())
	r.MustRegister(ApplyEdits())
}
