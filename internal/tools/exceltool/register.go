package exceltool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds the excel.* tool surface to the registry.
//
// Phase 0 of PLAN-workflow-surface narrowed this to the runScript escape
// hatch only. The host primitive constructors (WorkbookInfo, ReadRange, …)
// are kept compiling — they are reusable building blocks for the workflow
// tools to be added in Phase A — but no longer registered as MCP tools.
func Register(r *tools.Registry) {
	r.MustRegister(RunScript())
}
