package addintool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/dsbissett/office-addin-mcp/internal/webview2"
)

func TestRegister_AllTools(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)
	for _, name := range []string{
		"addin.ensureRunning",
		"addin.status",
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

// TestStatus_UnreachableEndpoint verifies the structured fallback path:
// when Discover fails, addin.status still returns OK with reachable=false
// and a recoveryHint pointing at addin.ensureRunning. Uses port 1 since
// no CDP server can possibly answer there.
func TestStatus_UnreachableEndpoint(t *testing.T) {
	res := runStatus(context.Background(), json.RawMessage(`{}`), &tools.RunEnv{
		Endpoint: webview2.Config{BrowserURL: "http://127.0.0.1:1"},
	})
	if res.Err != nil {
		t.Fatalf("expected OK envelope, got error %+v", res.Err)
	}
	out, ok := res.Data.(statusOutput)
	if !ok {
		t.Fatalf("Data type %T, want statusOutput", res.Data)
	}
	if out.Endpoint.Reachable {
		t.Error("Endpoint.Reachable = true, want false")
	}
	if out.Endpoint.Error == "" {
		t.Error("Endpoint.Error empty, want non-empty discovery failure")
	}
	if len(out.RecoveryHints) == 0 {
		t.Fatal("RecoveryHints empty, want at least one hint")
	}
	hint := strings.Join(out.RecoveryHints, " | ")
	if !strings.Contains(hint, "addin.ensureRunning") {
		t.Errorf("recoveryHints %q does not mention addin.ensureRunning", hint)
	}
}
