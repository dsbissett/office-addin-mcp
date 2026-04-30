package exceltool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds all excel.* tools to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(ReadRange())
	r.MustRegister(WriteRange())
	r.MustRegister(GetSelectedRange())
	r.MustRegister(SetSelectedRange())
	r.MustRegister(ListWorksheets())
	r.MustRegister(GetActiveWorksheet())
	r.MustRegister(ActivateWorksheet())
	r.MustRegister(CreateWorksheet())
	r.MustRegister(DeleteWorksheet())
	r.MustRegister(CreateTable())
	r.MustRegister(RunScript())
}
