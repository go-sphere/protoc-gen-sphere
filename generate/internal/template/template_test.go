package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewRendererIsIsolated(t *testing.T) {
	defaultRenderer, err := NewRenderer("")
	if err != nil {
		t.Fatalf("NewRenderer(default): %v", err)
	}

	path := filepath.Join(t.TempDir(), "custom.tmpl")
	if err := os.WriteFile(path, []byte("// custom {{.ServiceType}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	customRenderer, err := NewRenderer(path)
	if err != nil {
		t.Fatalf("NewRenderer(custom): %v", err)
	}

	desc := &ServiceDesc{ServiceType: "Greeter", Package: &PackageDesc{}}
	customOut, err := customRenderer.Execute(desc)
	if err != nil {
		t.Fatalf("custom Execute: %v", err)
	}
	if !strings.Contains(customOut, "// custom Greeter") {
		t.Errorf("custom template not applied, got %q", customOut)
	}

	defaultOut, err := defaultRenderer.Execute(desc)
	if err != nil {
		t.Fatalf("default Execute: %v", err)
	}
	if strings.Contains(defaultOut, "// custom") {
		t.Fatal("custom renderer must not mutate the embedded default renderer")
	}
}

func TestExecuteDoesNotMutateInput(t *testing.T) {
	renderer, err := NewRenderer("")
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	desc := &ServiceDesc{
		ServiceType: "Greeter",
		Methods:     []*MethodDesc{{Name: "Start"}},
		Package:     &PackageDesc{},
	}
	if _, err := renderer.Execute(desc); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if desc.MethodSets != nil {
		t.Errorf("Execute must not populate MethodSets, got %v", desc.MethodSets)
	}
}

func TestIndexMethodsLastWins(t *testing.T) {
	first := &MethodDesc{Name: "Start", Num: 0}
	second := &MethodDesc{Name: "Start", Num: 1}
	other := &MethodDesc{Name: "Stop"}
	sets := IndexMethods([]*MethodDesc{first, second, other})
	if len(sets) != 2 || sets["Start"] != second || sets["Stop"] != other {
		t.Errorf("unexpected index: %v", sets)
	}
}
