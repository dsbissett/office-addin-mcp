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

// boolPtrVal dereferences a *bool, failing the test when it is nil. Used to
// assert DestructiveHint values that the spec requires to be set explicitly.
func boolPtrVal(t *testing.T, p *bool, field string) bool {
	t.Helper()
	if p == nil {
		t.Fatalf("%s is nil, want an explicit value", field)
	}
	return *p
}

// TestToolAnnotations asserts the read-only vs mutating classification on a
// representative slice of this package's tools so an incorrect hint fails the
// build. Read-only probes must advertise ReadOnlyHint=true with a non-nil
// DestructiveHint=false; additive mutations must advertise ReadOnlyHint=false
// with DestructiveHint=false.
func TestToolAnnotations(t *testing.T) {
	readOnly := []struct {
		name string
		tool tools.Tool
	}{
		{"addin.status", Status()},
		{"addin.listTargets", ListTargets()},
		{"addin.contextInfo", ContextInfo()},
		{"addin.cfRuntimeInfo", CFRuntimeInfo()},
	}
	for _, tc := range readOnly {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.tool.Annotations
			if a == nil {
				t.Fatalf("%s: Annotations nil", tc.name)
			}
			if !a.ReadOnlyHint {
				t.Errorf("%s: ReadOnlyHint=false, want true", tc.name)
			}
			if boolPtrVal(t, a.DestructiveHint, tc.name+".DestructiveHint") {
				t.Errorf("%s: DestructiveHint=true, want false", tc.name)
			}
			if !a.IdempotentHint {
				t.Errorf("%s: IdempotentHint=false, want true", tc.name)
			}
		})
	}

	mutating := []struct {
		name string
		tool tools.Tool
	}{
		{"addin.ensureRunning", EnsureRunning()},
		{"addin.openDialog", OpenDialog()},
		{"addin.dialogClose", DialogClose()},
		{"addin.dialogSubscribe", DialogSubscribe()},
	}
	for _, tc := range mutating {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.tool.Annotations
			if a == nil {
				t.Fatalf("%s: Annotations nil", tc.name)
			}
			if a.ReadOnlyHint {
				t.Errorf("%s: ReadOnlyHint=true, want false (mutating)", tc.name)
			}
			// Every mutation in this package is additive, never destructive.
			if boolPtrVal(t, a.DestructiveHint, tc.name+".DestructiveHint") {
				t.Errorf("%s: DestructiveHint=true, want false (additive)", tc.name)
			}
		})
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
