package outlooktool

import (
	"encoding/json"
	"testing"
)

func TestArrayLen(t *testing.T) {
	cases := []struct {
		name string
		data any
		key  string
		want int
	}{
		{"three", map[string]any{"to": []any{1, 2, 3}}, "to", 3},
		{"empty-slice", map[string]any{"to": []any{}}, "to", 0},
		{"missing-key", map[string]any{"cc": []any{1}}, "to", 0},
		{"wrong-type", map[string]any{"to": "not-a-slice"}, "to", 0},
		{"not-a-map", "scalar", "to", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := arrayLen(tc.data, tc.key); got != tc.want {
				t.Errorf("arrayLen=%d want %d", got, tc.want)
			}
		})
	}
}

func TestStringField(t *testing.T) {
	cases := []struct {
		name string
		data any
		key  string
		want string
	}{
		{"present", map[string]any{"subject": "Hi"}, "subject", "Hi"},
		{"missing", map[string]any{"other": "x"}, "subject", ""},
		{"wrong-type", map[string]any{"subject": 42}, "subject", ""},
		{"not-a-map", []any{1}, "subject", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stringField(tc.data, tc.key); got != tc.want {
				t.Errorf("stringField=%q want %q", got, tc.want)
			}
		})
	}
}

// emptySelectorParams is exercised through the tools that decode it. This
// asserts the embedded selector round-trips a urlPattern.
func TestEmptySelectorParams_Selector(t *testing.T) {
	var p emptySelectorParams
	if err := json.Unmarshal([]byte(`{"urlPattern":"taskpane"}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Selector().URLPattern != "taskpane" {
		t.Errorf("urlPattern=%q", p.Selector().URLPattern)
	}
}
