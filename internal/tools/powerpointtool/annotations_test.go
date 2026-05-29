package powerpointtool

import (
	"strings"
	"testing"

	"github.com/dsbissett/office-addin-mcp/internal/tools"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// derefBool reports the value of a *bool, treating nil as false for the
// assertion helpers below.
func derefBool(p *bool) bool { return p != nil && *p }

func TestAnnotations_ReadOnlyTools(t *testing.T) {
	readOnly := []struct {
		name string
		tool tools.Tool
	}{
		{"powerpoint.readPresentation", ReadPresentation()},
		{"powerpoint.readSlides", ReadSlides()},
		{"powerpoint.readSlide", ReadSlide()},
		{"powerpoint.readSelection", ReadSelection()},
		{"powerpoint.discover", Discover()},
		{"powerpoint.query", Query()},
	}
	for _, tc := range readOnly {
		a := tc.tool.Annotations
		if a == nil {
			t.Errorf("%s: missing Annotations", tc.name)
			continue
		}
		if !a.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint = false, want true", tc.name)
		}
		if !a.IdempotentHint {
			t.Errorf("%s: IdempotentHint = false, want true", tc.name)
		}
		if derefBool(a.DestructiveHint) {
			t.Errorf("%s: DestructiveHint = true, want false", tc.name)
		}
	}
}

func TestAnnotations_DestructiveTools(t *testing.T) {
	destructive := []struct {
		name string
		tool tools.Tool
	}{
		{"powerpoint.runScript", RunScript()},
		{"powerpoint.rebuildSlideFromOutline", RebuildSlideFromOutline()},
	}
	for _, tc := range destructive {
		a := tc.tool.Annotations
		if a == nil {
			t.Errorf("%s: missing Annotations", tc.name)
			continue
		}
		if a.ReadOnlyHint {
			t.Errorf("%s: ReadOnlyHint = true, want false", tc.name)
		}
		if !derefBool(a.DestructiveHint) {
			t.Errorf("%s: DestructiveHint != true, want true", tc.name)
		}
	}
}

func TestAnnotations_RunScriptOpenWorld(t *testing.T) {
	a := RunScript().Annotations
	if a == nil || !derefBool(a.OpenWorldHint) {
		t.Errorf("powerpoint.runScript: OpenWorldHint != true, want true")
	}
}

func TestAnnotations_AdditiveMutation(t *testing.T) {
	// addSlide is an additive mutation: not read-only, not destructive.
	a := AddSlide().Annotations
	if a == nil {
		t.Fatal("powerpoint.addSlide: missing Annotations")
	}
	if a.ReadOnlyHint {
		t.Errorf("powerpoint.addSlide: ReadOnlyHint = true, want false")
	}
	if derefBool(a.DestructiveHint) {
		t.Errorf("powerpoint.addSlide: DestructiveHint = true, want false")
	}
}

// compileSchema compiles a draft-2020-12 JSON Schema string, failing the test
// if the schema itself is malformed.
func compileSchema(t *testing.T, name, schema string) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	if err := c.AddResource(name, strings.NewReader(schema)); err != nil {
		t.Fatalf("%s: add resource: %v", name, err)
	}
	s, err := c.Compile(name)
	if err != nil {
		t.Fatalf("%s: compile: %v", name, err)
	}
	return s
}

func TestDiscoverOutputSchema_ValidatesPayload(t *testing.T) {
	s := compileSchema(t, "powerpoint.discover.out", discoverOutputSchema)
	payload := map[string]any{
		"cached":      false,
		"filePath":    "Quarterly Deck",
		"fingerprint": "pp:s3:sh7",
		"title":       "Quarterly Deck",
		"slideCount":  float64(3),
		"shapeCount":  float64(7),
		"slides": []any{
			map[string]any{"id": "256", "index": float64(0), "shapeCount": float64(2)},
		},
	}
	if err := s.Validate(payload); err != nil {
		t.Fatalf("representative discover payload failed validation: %v", err)
	}

	// title may be null per the JS payload (pres.title || null).
	nullTitle := map[string]any{
		"cached":      true,
		"filePath":    "Deck",
		"fingerprint": "pp:s0:sh0",
		"title":       nil,
		"slideCount":  float64(0),
		"shapeCount":  float64(0),
		"slides":      []any{},
	}
	if err := s.Validate(nullTitle); err != nil {
		t.Fatalf("null-title discover payload failed validation: %v", err)
	}
}

func TestQueryOutputSchema_ValidatesPayload(t *testing.T) {
	s := compileSchema(t, "powerpoint.query.out", queryOutputSchema)
	payload := map[string]any{
		"slideCount": float64(3),
		"shapeCount": float64(12),
		"rows": []any{
			map[string]any{"name": "Title 1", "type": "GeometricShape"},
			map[string]any{"name": "Content"},
		},
		"count":   float64(2),
		"limited": false,
	}
	if err := s.Validate(payload); err != nil {
		t.Fatalf("representative query payload failed validation: %v", err)
	}
}
