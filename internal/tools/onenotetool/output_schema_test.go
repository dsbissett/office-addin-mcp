package onenotetool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// compileSchema compiles a raw JSON-Schema string, failing the test on error.
func compileSchema(t *testing.T, name string, raw json.RawMessage) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	if err := c.AddResource(name, strings.NewReader(string(raw))); err != nil {
		t.Fatalf("%s: add resource: %v", name, err)
	}
	sch, err := c.Compile(name)
	if err != nil {
		t.Fatalf("%s: compile: %v", name, err)
	}
	return sch
}

// validate unmarshals payload JSON and validates it against sch.
func validate(t *testing.T, sch *jsonschema.Schema, payload string) error {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	return sch.Validate(v)
}

func TestDiscoverOutputSchema_ValidatesRepresentativePayload(t *testing.T) {
	sch := compileSchema(t, "onenote.discover", Discover().OutputSchema)
	// Representative refresh payload: onenote_discover.js result merged with the
	// cache metadata officetool.RunDiscover adds (cached/filePath/fingerprint).
	payload := `{
      "cached": false,
      "filePath": "My Section",
      "fingerprint": "on:nb2:p3",
      "notebooks": [{"id": "nb1", "name": "Work"}, {"id": "nb2", "name": "Home"}],
      "activeSectionId": "sec1",
      "activeSectionName": "My Section",
      "pages": [{"id": "p1", "title": "Notes"}, {"id": "p2", "title": "Todo"}],
      "pageCount": 3
    }`
	if err := validate(t, sch, payload); err != nil {
		t.Errorf("representative discover payload failed validation: %v", err)
	}
}

func TestDiscoverOutputSchema_RejectsMissingCacheMeta(t *testing.T) {
	sch := compileSchema(t, "onenote.discover", Discover().OutputSchema)
	// Missing required cache-meta keys must fail — guards schema accuracy.
	if err := validate(t, sch, `{"notebooks": []}`); err == nil {
		t.Error("expected validation failure for payload missing cached/filePath/fingerprint")
	}
}

func TestQueryOutputSchema_ValidatesRepresentativePayload(t *testing.T) {
	sch := compileSchema(t, "onenote.query", Query().OutputSchema)
	// Representative onenote_query.js result.
	payload := `{
      "sectionId": "sec1",
      "sectionName": "My Section",
      "pageCount": 5,
      "rows": [{"id": "p1", "title": "Notes"}],
      "count": 1,
      "limited": false
    }`
	if err := validate(t, sch, payload); err != nil {
		t.Errorf("representative query payload failed validation: %v", err)
	}
}

func TestQueryOutputSchema_RejectsMissingRows(t *testing.T) {
	sch := compileSchema(t, "onenote.query", Query().OutputSchema)
	if err := validate(t, sch, `{"count": 0}`); err == nil {
		t.Error("expected validation failure for payload missing rows")
	}
}
