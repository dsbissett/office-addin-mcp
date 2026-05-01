package addintool

import (
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

func TestRegister_AllTools(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)
	for _, name := range []string{
		"addin.listTargets",
		"addin.contextInfo",
		"addin.openDialog",
		"addin.dialogClose",
		"addin.dialogSubscribe",
		"addin.cfRuntimeInfo",
	} {
		if _, ok := r.Get(name); !ok {
			t.Errorf("tool %s not registered", name)
		}
	}
}
