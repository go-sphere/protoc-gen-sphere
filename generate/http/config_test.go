package http

import "testing"

func TestParseGoIdent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantPath string
		wantName string
		wantErr  bool
	}{
		{name: "valid", input: "github.com/example/project/pkg;Message", wantPath: "github.com/example/project/pkg", wantName: "Message"},
		{name: "empty", input: "", wantErr: true},
		{name: "missing separator", input: "github.com/example/project/pkg/Message", wantErr: true},
		{name: "multiple separators", input: "github.com/example/project/pkg;Message;Other", wantErr: true},
		{name: "empty import path", input: ";Message", wantErr: true},
		{name: "empty identifier", input: "github.com/example/project/pkg;", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseGoIdent(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseGoIdent(%q) error = nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGoIdent(%q) error = %v", tt.input, err)
			}
			if gotPath := string(got.GoImportPath); gotPath != tt.wantPath {
				t.Errorf("GoImportPath = %q, want %q", gotPath, tt.wantPath)
			}
			if got.GoName != tt.wantName {
				t.Errorf("GoName = %q, want %q", got.GoName, tt.wantName)
			}
		})
	}
}
