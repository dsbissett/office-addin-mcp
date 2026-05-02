// Package addintool registers the addin.* tools that probe Office add-in
// surfaces and the Dialog API. Lifecycle helpers (addin.detect, addin.launch,
// addin.stop) live in the sibling lifecycletool package; addintool focuses on
// runtime introspection that requires a live CDP session.
package addintool

import "github.com/dsbissett/office-addin-mcp/internal/tools"

// Register adds every addin.* runtime tool to the registry. Lifecycle tools
// (addin.detect/launch/stop) are registered separately by lifecycletool.
func Register(r *tools.Registry) {
	r.MustRegister(EnsureRunning())
	r.MustRegister(Status())
	r.MustRegister(ListTargets())
	r.MustRegister(ContextInfo())
	r.MustRegister(OpenDialog())
	r.MustRegister(DialogClose())
	r.MustRegister(DialogSubscribe())
	r.MustRegister(CFRuntimeInfo())
}
