package cli

import (
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/browsertool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/cdptool"
)

// DefaultRegistry returns a registry populated with every built-in tool. New
// tool packages register themselves here during Phase 4+ rollout.
func DefaultRegistry() *tools.Registry {
	r := tools.NewRegistry()
	cdptool.Register(r)
	browsertool.Register(r)
	return r
}
