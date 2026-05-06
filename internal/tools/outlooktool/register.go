package outlooktool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds the outlook.* tool surface to the registry.
//
// Phase 0 of PLAN-workflow-surface narrowed this to the runScript escape
// hatch. Phase A adds workflow-shaped tools (draftReply). Primitive
// constructors stay in the package as reusable building blocks.
func Register(r *tools.Registry) {
	r.MustRegister(RunScript())
	r.MustRegister(DraftReply())
	r.MustRegister(Query())
	r.MustRegister(Discover())
}
