package exceltool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds all excel.* tools to the registry.
func Register(r *tools.Registry) {
	// Workbook-scoped
	r.MustRegister(WorkbookInfo())
	r.MustRegister(CalculationState())
	r.MustRegister(ListNamedItems())
	r.MustRegister(CustomXMLParts())
	r.MustRegister(SettingsGet())

	// Worksheet-scoped
	r.MustRegister(ListWorksheets())
	r.MustRegister(GetActiveWorksheet())
	r.MustRegister(WorksheetInfo())
	r.MustRegister(ActivateWorksheet())
	r.MustRegister(CreateWorksheet())
	r.MustRegister(DeleteWorksheet())
	r.MustRegister(ListComments())
	r.MustRegister(ListShapes())

	// Range-scoped
	r.MustRegister(ReadRange())
	r.MustRegister(WriteRange())
	r.MustRegister(GetSelectedRange())
	r.MustRegister(SetSelectedRange())
	r.MustRegister(ActiveRange())
	r.MustRegister(UsedRange())
	r.MustRegister(RangeProperties())
	r.MustRegister(RangeFormulas())
	r.MustRegister(RangeSpecialCells())
	r.MustRegister(FindInRange())
	r.MustRegister(ListConditionalFormats())
	r.MustRegister(ListDataValidations())

	// Tables
	r.MustRegister(CreateTable())
	r.MustRegister(ListTables())
	r.MustRegister(TableInfo())
	r.MustRegister(TableRows())
	r.MustRegister(TableFilters())

	// Charts
	r.MustRegister(ListCharts())
	r.MustRegister(ChartInfo())
	r.MustRegister(ChartImage())

	// PivotTables
	r.MustRegister(ListPivotTables())
	r.MustRegister(PivotTableInfo())
	r.MustRegister(PivotTableValues())

	// Escape hatch
	r.MustRegister(RunScript())
}
