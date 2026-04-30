package cli

import (
	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/tools/browsertool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/cdptool"
	"github.com/dsbissett/office-addin-mcp/internal/tools/exceltool"
)

// DefaultRegistry returns a registry populated with every built-in tool.
func DefaultRegistry() *tools.Registry {
	r := tools.NewRegistry()
	cdptool.Register(r)
	browsertool.Register(r)
	exceltool.Register(r)
	return r
}
