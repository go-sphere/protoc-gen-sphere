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
	if desc.DistinctMethods != nil {
		t.Errorf("Execute must not populate DistinctMethods, got %v", desc.DistinctMethods)
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

func TestDistinctMethodsDeclarationOrder(t *testing.T) {
	// Additional bindings precede the primary rule (Start0, Start1), so the
	// Name keeps its first position but takes the last descriptor, matching
	// IndexMethods.
	watch := &MethodDesc{Name: "Watch"}
	start0 := &MethodDesc{Name: "Start", Num: 0}
	start1 := &MethodDesc{Name: "Start", Num: 1}
	chat := &MethodDesc{Name: "Chat"}
	methods := []*MethodDesc{watch, start0, start1, chat}

	got := DistinctMethods(methods)
	want := []*MethodDesc{watch, start1, chat}
	if len(got) != len(want) {
		t.Fatalf("DistinctMethods() returned %d entries, want %d", len(got), len(want))
	}
	sets := IndexMethods(methods)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("DistinctMethods()[%d] = %s#%d, want %s#%d", i, got[i].Name, got[i].Num, want[i].Name, want[i].Num)
		}
		if sets[got[i].Name] != got[i] {
			t.Errorf("DistinctMethods()[%d] differs from IndexMethods for %s", i, got[i].Name)
		}
	}
	if got := DistinctMethods(nil); len(got) != 0 {
		t.Errorf("DistinctMethods(nil) = %v, want empty", got)
	}
}
