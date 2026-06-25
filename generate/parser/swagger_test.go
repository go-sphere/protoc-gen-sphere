package parser

import "testing"

func TestSwaggerDescription(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "  \n  ", ""},
		{"single line", " hello ", "hello"},
		{"multi line joined with comma", "line1\nline2\nline3", "line1,line2,line3"},
		{"trailing newline trimmed", "line1\nline2\n", "line1,line2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := swaggerDescription(tt.in); got != tt.want {
				t.Errorf("swaggerDescription(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
