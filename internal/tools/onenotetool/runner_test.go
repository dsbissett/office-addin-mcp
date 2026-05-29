package onenotetool

import "testing"

func TestArrayLen(t *testing.T) {
	cases := []struct {
		name string
		data any
		key  string
		want int
	}{
		{"non-map", "not a map", "rows", 0},
		{"missing key", map[string]any{"other": []any{1}}, "rows", 0},
		{"wrong type", map[string]any{"rows": "not an array"}, "rows", 0},
		{"empty array", map[string]any{"rows": []any{}}, "rows", 0},
		{"counts", map[string]any{"rows": []any{1, 2, 3}}, "rows", 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := arrayLen(c.data, c.key); got != c.want {
				t.Errorf("arrayLen(%v,%q)=%d, want %d", c.data, c.key, got, c.want)
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
		{"non-map", 42, "title", ""},
		{"missing key", map[string]any{"other": "x"}, "title", ""},
		{"wrong type", map[string]any{"title": 99}, "title", ""},
		{"present", map[string]any{"title": "Hello"}, "title", "Hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stringField(c.data, c.key); got != c.want {
				t.Errorf("stringField(%v,%q)=%q, want %q", c.data, c.key, got, c.want)
			}
		})
	}
}
