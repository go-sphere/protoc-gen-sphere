package http

import (
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
)

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

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultConfig().Validate() error = %v", err)
	}
	if !cfg.OmitEmpty {
		t.Error("DefaultConfig().OmitEmpty = false, want true")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "router type", mutate: func(c *Config) { c.RouterType = protogen.GoIdent{} }},
		{name: "context type", mutate: func(c *Config) { c.ContextType = protogen.GoIdent{} }},
		{name: "handler type", mutate: func(c *Config) { c.HandlerType = protogen.GoIdent{} }},
		{name: "error response type", mutate: func(c *Config) { c.ErrorRespType = protogen.GoIdent{} }},
		{name: "data response type", mutate: func(c *Config) { c.DataRespType = protogen.GoIdent{} }},
		{name: "server handler func", mutate: func(c *Config) { c.ServerHandlerFunc = protogen.GoIdent{} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
	if err := (*Config)(nil).Validate(); err == nil {
		t.Fatal("nil Config.Validate() error = nil")
	}
}
