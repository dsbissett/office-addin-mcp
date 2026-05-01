package inspecttool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds the page.snapshot / page.screenshot / page.waitFor /
// page.evaluate / page.consoleLog / page.networkLog / page.networkBody
// tools to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(Snapshot())
	r.MustRegister(Screenshot())
	r.MustRegister(WaitFor())
	r.MustRegister(Evaluate())
	r.MustRegister(ConsoleLog())
	r.MustRegister(NetworkLog())
	r.MustRegister(NetworkBody())
}
