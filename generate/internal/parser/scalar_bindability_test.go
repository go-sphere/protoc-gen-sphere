package parser

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	bindingpb "github.com/go-sphere/binding/sphere/binding"
	"github.com/go-sphere/protoc-gen-sphere/generate/internal/testutil"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

// Shared with protoc-gen-sphere-binding; the descriptor is built by `make
// testdata` from generate/http/testdata/proto/scalar_bindability.proto.
const (
	scalarBindabilityPB    = "../../http/testdata/pb/scalar_bindability.pb"
	scalarBindabilityTable = "../../http/testdata/golden/scalar_bindability.golden"
)

// TestScalarBindabilityContract runs the real per-location collectors
// (QueryParams/URIParams/HeaderParams/FormParams) over the shared
// scalar-bindability fixture and compares every (location, field) decision with
// the shared expected table.
//
// The fixture proto and the table are kept byte-identical with
// protoc-gen-sphere-binding (generate/binding/testdata/), whose tagger runs the
// same table through its own copy of isScalarBindable/fieldKindDesc. The table
// is a hand-maintained contract, not a regenerated snapshot: change it in both
// repositories together, never from one implementation's output alone.
func TestScalarBindabilityContract(t *testing.T) {
	set := testutil.LoadDescriptorSet(t, scalarBindabilityPB)
	plugin := testutil.MustCreatePlugin(t, set, "scalar_bindability.proto")
	file := testutil.FileToGenerate(t, plugin)

	var got []string
	for _, svc := range file.Services {
		for _, m := range svc.Methods {
			loc := proto.GetExtension(m.Input.Desc.Options(), bindingpb.E_DefaultLocation).(bindingpb.BindingLocation)
			locName, collect := scalarBindabilityCollector(loc)
			if collect == nil {
				t.Fatalf("method %s: input has no query/uri/header/form default_location", m.Desc.Name())
			}
			for _, field := range m.Input.Fields {
				got = append(got, fmt.Sprintf("%s %s %s %s", locName, field.Desc.Name(), fieldKindDesc(field),
					scalarBindabilityDecision(t, m, field, collect)))
			}
		}
	}

	want := readScalarBindabilityTable(t, scalarBindabilityTable)
	if !slices.Equal(want, got) {
		t.Errorf("scalar bindability decisions diverge from the shared table:\n%s\nfull table from this implementation:\n%s",
			firstRowDiff(want, got), strings.Join(got, "\n"))
	}
}

type paramsCollector func(*protogen.Method, string) ([]ParamsField, error)

func scalarBindabilityCollector(loc bindingpb.BindingLocation) (string, paramsCollector) {
	switch loc {
	case bindingpb.BindingLocation_BINDING_LOCATION_QUERY:
		return "query", func(m *protogen.Method, _ string) ([]ParamsField, error) { return QueryParams(m, "POST", nil) }
	case bindingpb.BindingLocation_BINDING_LOCATION_URI:
		return "uri", func(m *protogen.Method, name string) ([]ParamsField, error) { return URIParams(m, "/x/:"+name) }
	case bindingpb.BindingLocation_BINDING_LOCATION_HEADER:
		return "header", func(m *protogen.Method, _ string) ([]ParamsField, error) { return HeaderParams(m) }
	case bindingpb.BindingLocation_BINDING_LOCATION_FORM:
		return "form", func(m *protogen.Method, _ string) ([]ParamsField, error) { return FormParams(m) }
	}
	return "", nil
}

// scalarBindabilityDecision runs collect on a view of m whose input holds only
// field, so each decision is independent of the other fields.
//
// Real oneof members are detached from their oneof first: this plugin never
// binds them outside the JSON body (the collectors skip them and
// OneofBindingIssues warns), so their rows pin the shared predicate that would
// apply if they were bound, which is what protoc-gen-sphere-binding enforces.
func scalarBindabilityDecision(t *testing.T, m *protogen.Method, field *protogen.Field, collect paramsCollector) string {
	t.Helper()
	f := *field
	if isRealOneofMember(&f) {
		f.Oneof = nil
	}
	input := *m.Input
	input.Fields = []*protogen.Field{&f}
	method := *m
	method.Input = &input

	params, err := collect(&method, string(f.Desc.Name()))
	if err != nil {
		if !strings.Contains(err.Error(), "only scalar types") {
			t.Fatalf("%s.%s: unexpected error: %v", m.Input.Desc.Name(), f.Desc.Name(), err)
		}
		return "reject"
	}
	if len(params) != 1 || params[0].Field != &f {
		t.Fatalf("%s.%s: accepted but not collected (got %d params)", m.Input.Desc.Name(), f.Desc.Name(), len(params))
	}
	return "accept"
}

// readScalarBindabilityTable returns the non-comment rows of the shared table
// with runs of whitespace collapsed, so columns may be aligned freely.
func readScalarBindabilityTable(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s (run `make testdata` for the descriptor): %v", path, err)
	}
	var rows []string
	for line := range strings.Lines(string(data)) {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		rows = append(rows, strings.Join(fields, " "))
	}
	return rows
}

func firstRowDiff(want, got []string) string {
	for i := 0; i < len(want) && i < len(got); i++ {
		if want[i] != got[i] {
			return fmt.Sprintf("first difference at row %d:\n  want: %q\n  got:  %q", i+1, want[i], got[i])
		}
	}
	return fmt.Sprintf("row count differs: want %d, got %d", len(want), len(got))
}
