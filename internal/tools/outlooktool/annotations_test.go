package outlooktool

import (
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
)

// TestAnnotationsByTool asserts the MCP hint flags on representative read-only,
// destructive, and additive-mutating tool constructors. It also verifies that
// every registered tool carries annotations, so a new constructor that forgets
// them fails the build.
func TestAnnotationsByTool(t *testing.T) {
	cases := []struct {
		name        string
		tool        tools.Tool
		readOnly    bool
		idempotent  bool
		destructive *bool
		openWorld   *bool
	}{
		{"readItem", ReadItem(), true, true, tools.BoolPtr(false), nil},
		{"getBody", GetBody(), true, true, tools.BoolPtr(false), nil},
		{"getSubject", GetSubject(), true, true, tools.BoolPtr(false), nil},
		{"getRecipients", GetRecipients(), true, true, tools.BoolPtr(false), nil},
		{"query", Query(), true, true, tools.BoolPtr(false), nil},
		{"discover", Discover(), true, true, tools.BoolPtr(false), nil},
		{"setBody", SetBody(), false, true, tools.BoolPtr(true), nil},
		{"setSubject", SetSubject(), false, true, tools.BoolPtr(true), nil},
		{"runScript", RunScript(), false, false, tools.BoolPtr(true), tools.BoolPtr(true)},
		{"draftReply", DraftReply(), false, true, tools.BoolPtr(true), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.tool.Annotations
			if a == nil {
				t.Fatalf("%s: Annotations is nil", tc.name)
			}
			if a.ReadOnlyHint != tc.readOnly {
				t.Errorf("%s: ReadOnlyHint=%v want %v", tc.name, a.ReadOnlyHint, tc.readOnly)
			}
			if a.IdempotentHint != tc.idempotent {
				t.Errorf("%s: IdempotentHint=%v want %v", tc.name, a.IdempotentHint, tc.idempotent)
			}
			assertBoolPtr(t, tc.name+".DestructiveHint", a.DestructiveHint, tc.destructive)
			assertBoolPtr(t, tc.name+".OpenWorldHint", a.OpenWorldHint, tc.openWorld)
		})
	}
}

// TestEveryToolHasAnnotations guards against a future constructor shipping
// without hint flags.
func TestEveryToolHasAnnotations(t *testing.T) {
	r := tools.NewRegistry()
	Register(r)
	for _, tool := range r.List() {
		if tool.Annotations == nil {
			t.Errorf("tool %q has nil Annotations", tool.Name)
		}
	}
}

func assertBoolPtr(t *testing.T, label string, got, want *bool) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s=%v want nil", label, *got)
	case want != nil && got == nil:
		t.Errorf("%s=nil want %v", label, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s=%v want %v", label, *got, *want)
	}
}
