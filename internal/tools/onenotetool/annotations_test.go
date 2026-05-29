package onenotetool

import (
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// boolPtrVal dereferences an *bool annotation field, failing the test when nil.
func boolPtrVal(t *testing.T, name string, p *bool) bool {
	t.Helper()
	if p == nil {
		t.Fatalf("%s: expected non-nil *bool annotation", name)
	}
	return *p
}

func TestAnnotations_ReadOnlyTools(t *testing.T) {
	readOnly := []tools.Tool{
		ReadNotebooks(), ReadSections(), ReadPages(), ReadPage(), Discover(), Query(),
	}
	for _, tool := range readOnly {
		a := tool.Annotations
		if a == nil {
			t.Fatalf("%s: expected annotations", tool.Name)
		}
		if !a.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint should be true", tool.Name)
		}
		if !a.IdempotentHint {
			t.Errorf("%s: IdempotentHint should be true", tool.Name)
		}
		if boolPtrVal(t, tool.Name, a.DestructiveHint) {
			t.Errorf("%s: DestructiveHint should be false", tool.Name)
		}
	}
}

func TestAnnotations_RunScriptIsDestructiveOpenWorld(t *testing.T) {
	a := RunScript().Annotations
	if a == nil {
		t.Fatal("runScript: expected annotations")
	}
	if a.ReadOnlyHint {
		t.Error("runScript: ReadOnlyHint should be false")
	}
	if !boolPtrVal(t, "onenote.runScript", a.DestructiveHint) {
		t.Error("runScript: DestructiveHint should be true")
	}
	if !boolPtrVal(t, "onenote.runScript", a.OpenWorldHint) {
		t.Error("runScript: OpenWorldHint should be true")
	}
}

func TestAnnotations_AdditiveMutationsNonDestructive(t *testing.T) {
	additive := []tools.Tool{AddPage(), AppendToPage()}
	for _, tool := range additive {
		a := tool.Annotations
		if a == nil {
			t.Fatalf("%s: expected annotations", tool.Name)
		}
		if a.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint should be false", tool.Name)
		}
		if boolPtrVal(t, tool.Name, a.DestructiveHint) {
			t.Errorf("%s: DestructiveHint should be false (additive)", tool.Name)
		}
		if a.IdempotentHint {
			t.Errorf("%s: IdempotentHint should be false (each call adds content)", tool.Name)
		}
	}
}
