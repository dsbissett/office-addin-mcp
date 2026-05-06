// Package macrotool registers the macro.* tools for recording and replaying
// macro sequences (Phase E).
package macrotool

import (
	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// Register adds macro.record_start, macro.record_stop, and all loaded macro
// tools to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(RecordStart())
	r.MustRegister(RecordStop())
}
