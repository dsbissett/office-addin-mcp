package lifecycletool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds addin.detect / addin.launch / addin.stop to the registry.
func Register(r *tools.Registry) {
	r.MustRegister(Detect())
	r.MustRegister(Launch())
	r.MustRegister(Stop())
}
