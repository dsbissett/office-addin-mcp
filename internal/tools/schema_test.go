package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

const exampleSchema = `{
  "type": "object",
  "properties": {
    "expression": {"type": "string", "minLength": 1}
  },
  "required": ["expression"],
  "additionalProperties": false
}`

func TestSchema_NilForEmpty(t *testing.T) {
	s, err := compileSchema("empty", nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if s != nil {
		t.Fatal("expected nil schema for empty input")
	}
	if err := validateParams(s, []byte(`{"anything":1}`)); err != nil {
		t.Errorf("nil schema should pass everything, got %v", err)
	}
}

func TestSchema_ValidateRequired(t *testing.T) {
	s, err := compileSchema("ex", json.RawMessage(exampleSchema))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := validateParams(s, []byte(`{}`)); err == nil {
		t.Fatal("expected error for missing required field")
	}
	if err := validateParams(s, []byte(`{"expression":"1+1"}`)); err != nil {
		t.Errorf("expected pass, got %v", err)
	}
}

func TestSchema_ValidateAdditionalProperty(t *testing.T) {
	s, err := compileSchema("ex", json.RawMessage(exampleSchema))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	err = validateParams(s, []byte(`{"expression":"x","oops":true}`))
	if err == nil || !strings.Contains(err.Error(), "additional") {
		t.Errorf("expected additional-property error, got %v", err)
	}
}

func TestSchema_NullParamsTreatedAsEmptyObject(t *testing.T) {
	s, err := compileSchema("ex", json.RawMessage(`{"type":"object"}`))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := validateParams(s, []byte("null")); err != nil {
		t.Errorf("null params should validate as empty object, got %v", err)
	}
	if err := validateParams(s, nil); err != nil {
		t.Errorf("nil params should validate as empty object, got %v", err)
	}
}

func TestSchema_BadJSONReportsClearly(t *testing.T) {
	s, _ := compileSchema("ex", json.RawMessage(`{"type":"object"}`))
	err := validateParams(s, []byte(`{not json`))
	if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("expected JSON decode error, got %v", err)
	}
}
