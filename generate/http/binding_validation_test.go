package http

import (
	"strings"
	"testing"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/parser"
	"github.com/go-sphere/protoc-gen-sphere/generate/internal/testutil"
	"google.golang.org/protobuf/compiler/protogen"
)

// TestBindingLocationKindValidation verifies BUG-47: fields whose type cannot be
// decoded from a single string token (map / message / bytes) must be rejected at
// generation time when they are bound to QUERY / URI / HEADER, instead of
// silently emitting a binding the runtime cannot satisfy.
func TestBindingLocationKindValidation(t *testing.T) {
	set := testutil.LoadDescriptorSet(t, "testdata/pb/invalid_binding.pb")
	plugin := testutil.MustCreatePlugin(t, set, "invalid_binding.proto")
	file := testutil.FileToGenerate(t, plugin)

	methods := map[string]*protogen.Method{}
	for _, svc := range file.Services {
		for _, m := range svc.Methods {
			methods[m.GoName] = m
		}
	}

	t.Run("message bound to QUERY", func(t *testing.T) {
		m := methods["QueryMessage"]
		if m == nil {
			t.Fatal("method QueryMessage not found")
		}
		_, err := parser.QueryParams(m, "POST", nil)
		if err == nil {
			t.Fatal("expected error for message field bound to QUERY, got nil")
		}
		if !strings.Contains(err.Error(), "QUERY") {
			t.Errorf("error should mention QUERY: %v", err)
		}
	})

	t.Run("message bound to URI", func(t *testing.T) {
		m := methods["UriMessage"]
		if m == nil {
			t.Fatal("method UriMessage not found")
		}
		_, err := parser.URIParams(m, "/api/u/:inner")
		if err == nil {
			t.Fatal("expected error for message field bound to URI, got nil")
		}
		if !strings.Contains(err.Error(), "URI") {
			t.Errorf("error should mention URI: %v", err)
		}
	})

	t.Run("map bound to HEADER", func(t *testing.T) {
		m := methods["HeaderMap"]
		if m == nil {
			t.Fatal("method HeaderMap not found")
		}
		_, err := parser.HeaderParams(m)
		if err == nil {
			t.Fatal("expected error for map field bound to HEADER, got nil")
		}
		if !strings.Contains(err.Error(), "HEADER") {
			t.Errorf("error should mention HEADER: %v", err)
		}
	})

	t.Run("whole file generation fails fast", func(t *testing.T) {
		// GenerateFile must surface the first binding error rather than emitting a
		// partial file.
		if _, err := GenerateFile(plugin, file, DefaultConfig()); err == nil {
			t.Fatal("expected GenerateFile to fail on invalid binding, got nil")
		}
	})
}
