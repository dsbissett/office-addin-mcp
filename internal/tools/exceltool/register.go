package exceltool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds the excel.* tool surface to the registry.
//
// Phase 0 of PLAN-workflow-surface narrowed this to the runScript escape
// hatch. Phase A adds workflow-shaped tools (tabulateRegion, applyDiff,
// summarizeWorkbook). The remaining primitive constructors (WorkbookInfo,
// ReadRange, …) stay in the package as reusable building blocks but are not
// registered as MCP tools.
func Register(r *tools.Registry) {
	r.MustRegister(RunScript())
	r.MustRegister(TabulateRegion())
	r.MustRegister(ApplyDiff())
	r.MustRegister(SummarizeWorkbook())
}
