package resources

import (
	"testing"
)

func TestParseURI(t *testing.T) {
	tests := []struct {
		name      string
		uri       string
		want      *ParsedURI
		wantError bool
	}{
		{
			name: "excel range",
			uri:  "office://excel/Book1/Sheet1!A1:D20",
			want: &ParsedURI{
				Host:  "excel",
				Parts: []string{"Book1", "Sheet1!A1:D20"},
				Raw:   "office://excel/Book1/Sheet1!A1:D20",
			},
		},
		{
			name: "word bookmark",
			uri:  "office://word/mydoc/bookmark/intro",
			want: &ParsedURI{
				Host:  "word",
				Parts: []string{"mydoc", "bookmark", "intro"},
				Raw:   "office://word/mydoc/bookmark/intro",
			},
		},
		{
			name: "outlook folder",
			uri:  "office://outlook/inbox",
			want: &ParsedURI{
				Host:  "outlook",
				Parts: []string{"inbox"},
				Raw:   "office://outlook/inbox",
			},
		},
		{
			name: "powerpoint slide",
			uri:  "office://pp/deck1/slide2",
			want: &ParsedURI{
				Host:  "pp",
				Parts: []string{"deck1", "slide2"},
				Raw:   "office://pp/deck1/slide2",
			},
		},
		{
			name: "onenote page",
			uri:  "office://onenote/notebook/section/page",
			want: &ParsedURI{
				Host:  "onenote",
				Parts: []string{"notebook", "section", "page"},
				Raw:   "office://onenote/notebook/section/page",
			},
		},
		{
			name:      "invalid scheme",
			uri:       "http://excel/Book1",
			wantError: true,
		},
		{
			name:      "missing host",
			uri:       "office:///Book1",
			wantError: true,
		},
		{
			name:      "unknown host",
			uri:       "office://sheets/Book1",
			wantError: true,
		},
		{
			name: "case insensitive host",
			uri:  "office://EXCEL/Book1/Sheet1",
			want: &ParsedURI{
				Host:  "excel",
				Parts: []string{"Book1", "Sheet1"},
				Raw:   "office://EXCEL/Book1/Sheet1",
			},
		},
		{
			name: "host only",
			uri:  "office://excel",
			want: &ParsedURI{
				Host:  "excel",
				Parts: []string{},
				Raw:   "office://excel",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseURI(tt.uri)
			if (err != nil) != tt.wantError {
				t.Fatalf("ParseURI(%q) error = %v, wantError %v", tt.uri, err, tt.wantError)
			}
			if tt.wantError {
				return
			}
			if got.Host != tt.want.Host {
				t.Errorf("Host = %q, want %q", got.Host, tt.want.Host)
			}
			if len(got.Parts) != len(tt.want.Parts) {
				t.Errorf("Parts length = %d, want %d", len(got.Parts), len(tt.want.Parts))
			}
			for i, part := range got.Parts {
				if part != tt.want.Parts[i] {
					t.Errorf("Parts[%d] = %q, want %q", i, part, tt.want.Parts[i])
				}
			}
		})
	}
}

func TestURIRoundTrip(t *testing.T) {
	tests := []string{
		"office://excel/Book1/Sheet1!A1:D20",
		"office://word/mydoc/bookmark/intro",
		"office://outlook/inbox",
		"office://pp/deck1/slide2",
		"office://onenote/notebook/section/page",
	}

	for _, uri := range tests {
		t.Run(uri, func(t *testing.T) {
			parsed, err := ParseURI(uri)
			if err != nil {
				t.Fatalf("ParseURI error: %v", err)
			}
			result := parsed.String()
			if result != uri {
				t.Errorf("String() = %q, want %q", result, uri)
			}
		})
	}
}
