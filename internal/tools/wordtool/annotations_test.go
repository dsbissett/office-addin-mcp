package wordtool

import (
	"bytes"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// boolPtrEq reports whether a *bool equals want.
func boolPtrEq(got *bool, want bool) bool {
	return got != nil && *got == want
}

// TestAnnotations asserts representative word.* tools carry the expected MCP
// annotation hints so a regression in the constructors fails the build.
func TestAnnotations(t *testing.T) {
	cases := []struct {
		name        string
		tool        tools.Tool
		readOnly    bool
		idempotent  bool
		destructive bool
		openWorld   *bool
	}{
		// Read-only: no document/app mutation.
		{"word.readBody", ReadBody(), true, true, false, nil},
		{"word.readParagraphs", ReadParagraphs(), true, true, false, nil},
		{"word.readSelection", ReadSelection(), true, true, false, nil},
		{"word.searchText", SearchText(), true, true, false, nil},
		{"word.readProperties", ReadProperties(), true, true, false, nil},
		{"word.discover", Discover(), true, true, false, nil},
		// Destructive mutations: replace/overwrite or run arbitrary code.
		{"word.writeBody", WriteBody(), false, false, true, nil},
		{"word.applyEdits", ApplyEdits(), false, false, true, nil},
		{"word.runScript", RunScript(), false, false, true, tools.BoolPtr(true)},
		// Additive mutation.
		{"word.insertParagraph", InsertParagraph(), false, false, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.tool.Annotations
			if a == nil {
				t.Fatal("Annotations is nil")
			}
			if a.ReadOnlyHint != tc.readOnly {
				t.Errorf("ReadOnlyHint=%v, want %v", a.ReadOnlyHint, tc.readOnly)
			}
			if a.IdempotentHint != tc.idempotent {
				t.Errorf("IdempotentHint=%v, want %v", a.IdempotentHint, tc.idempotent)
			}
			if !boolPtrEq(a.DestructiveHint, tc.destructive) {
				t.Errorf("DestructiveHint=%v, want %v", a.DestructiveHint, tc.destructive)
			}
			if tc.openWorld == nil {
				if a.OpenWorldHint != nil {
					t.Errorf("OpenWorldHint=%v, want nil", *a.OpenWorldHint)
				}
			} else if !boolPtrEq(a.OpenWorldHint, *tc.openWorld) {
				t.Errorf("OpenWorldHint=%v, want %v", a.OpenWorldHint, *tc.openWorld)
			}
		})
	}
}

// TestEveryToolHasAnnotations guards against a new constructor shipping without
// the required MCP annotation metadata.
func TestEveryToolHasAnnotations(t *testing.T) {
	all := []tools.Tool{
		ReadBody(), WriteBody(), ReadParagraphs(), InsertParagraph(),
		ReadSelection(), SearchText(), ReadProperties(),
		RunScript(), ApplyEdits(), Discover(),
	}
	for _, tool := range all {
		if tool.Annotations == nil {
			t.Errorf("%s: Annotations is nil", tool.Name)
		}
	}
}

// TestDiscoverOutputSchema compiles word.discover's OutputSchema and validates a
// representative success payload against it, so an inaccurate schema fails the
// build. The payload mirrors what RunDiscover returns: the cache metadata keys
// (cached, filePath, fingerprint) merged over the host discovery snapshot.
func TestDiscoverOutputSchema(t *testing.T) {
	schema := Discover().OutputSchema
	if len(schema) == 0 {
		t.Fatal("word.discover has no OutputSchema")
	}
	c := jsonschema.NewCompiler()
	const url = "mem://word.discover.result.json"
	if err := c.AddResource(url, bytes.NewReader(schema)); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	compiled, err := c.Compile(url)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	valid := map[string]any{
		"cached":          false,
		"filePath":        `C:\docs\report.docx`,
		"fingerprint":     "fp-1",
		"title":           "Report",
		"author":          "Jane",
		"wordCount":       float64(1200),
		"sections":        []any{map[string]any{"title": "Intro"}},
		"contentControls": []any{},
	}
	if err := compiled.Validate(valid); err != nil {
		t.Errorf("representative payload should validate: %v", err)
	}

	// A payload missing a required cache-metadata key must be rejected — this
	// asserts the schema actually constrains the top level.
	missing := map[string]any{"title": "Report"}
	if err := compiled.Validate(missing); err == nil {
		t.Error("payload missing required keys should fail validation")
	}
}
