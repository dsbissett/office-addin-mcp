package tools

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// compileSchema turns a JSON Schema in raw bytes into a compiled validator.
// An empty schema is treated as "no validation" and returns (nil, nil).
func compileSchema(name string, raw json.RawMessage) (*jsonschema.Schema, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	compiler := jsonschema.NewCompiler()
	url := "mem://" + name + ".schema.json"
	if err := compiler.AddResource(url, bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("add resource: %w", err)
	}
	sch, err := compiler.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	return sch, nil
}

// validateParams validates raw params against a compiled schema. nil schema
// short-circuits to success. The error message is shaped for envelopes — keep
// it terse and human-readable.
func validateParams(s *jsonschema.Schema, raw json.RawMessage) error {
	if s == nil {
		return nil
	}
	var v any
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		v = map[string]any{}
	} else if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return s.Validate(v)
}
