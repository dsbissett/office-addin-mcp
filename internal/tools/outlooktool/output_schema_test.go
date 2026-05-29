package outlooktool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// compileSchema compiles a raw JSON Schema string, failing the test if it is
// not a valid draft 2020-12 schema.
func compileSchema(t *testing.T, name, raw string) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	if err := c.AddResource(name, strings.NewReader(raw)); err != nil {
		t.Fatalf("%s: add resource: %v", name, err)
	}
	s, err := c.Compile(name)
	if err != nil {
		t.Fatalf("%s: compile: %v", name, err)
	}
	return s
}

// validate decodes a JSON payload and validates it against the compiled schema.
func validate(t *testing.T, s *jsonschema.Schema, payload string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(payload), &v); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if err := s.Validate(v); err != nil {
		t.Fatalf("payload failed schema validation: %v", err)
	}
}

// TestQueryOutputSchema_ValidatesSuccessPayload mirrors the shape produced by
// internal/js/outlook_query.js (folder/rows/count/limited/note).
func TestQueryOutputSchema_ValidatesSuccessPayload(t *testing.T) {
	s := compileSchema(t, "outlook.query.out", queryOutputSchema)
	validate(t, s, `{
	  "folder": "currentItem",
	  "rows": [{"subject": "Re: budget", "from": "a@example.com", "to": ["b@example.com"]}],
	  "count": 1,
	  "limited": false,
	  "note": "Outlook query reads the active item context."
	}`)
	// Empty result set is also valid.
	validate(t, s, `{"folder": "currentItem", "rows": [], "count": 0, "limited": false}`)
}

// TestDiscoverOutputSchema_ValidatesSuccessPayload mirrors the shape produced
// by RunDiscover/withCacheMeta merged with internal/js/outlook_discover.js.
func TestDiscoverOutputSchema_ValidatesSuccessPayload(t *testing.T) {
	s := compileSchema(t, "outlook.discover.out", discoverOutputSchema)
	validate(t, s, `{
	  "cached": false,
	  "filePath": "u@example.com",
	  "fingerprint": "outlook:uu@example.com:inone",
	  "userEmail": "u@example.com",
	  "userName": "User One",
	  "hostMode": "messageCompose",
	  "activeItemId": null,
	  "conversationId": null
	}`)
	// Cache-hit minimal shape (only the meta fields guaranteed by withCacheMeta).
	validate(t, s, `{"cached": true, "filePath": "u@example.com", "fingerprint": "fp1"}`)
}
