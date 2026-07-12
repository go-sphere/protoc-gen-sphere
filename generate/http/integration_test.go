package http

import (
	goparser "go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/testutil"
)

// TestIntegrationOutput is the cross-plugin integration test (OPT-02). It runs
// the http generator against a fixture that combines every binding shape that
// historically only broke when the plugins were composed (nested message + map
// + oneof in the JSON body, a multipart form upload, and well-known types as
// query params) and asserts:
//
//   - the generated file is syntactically valid Go (the minimal "compiles"
//     gate, since go/format guarantees nothing about the actual template body);
//   - FORM-bound requests actually emit a BindForm call and a formData Swagger
//     param (the "binding tags a field but http never consumes it" bug);
//   - well-known types used as query params render as scalar Swagger types
//     rather than Go package type names (BUG-48);
//   - each binding location produces the matching Bind* call.
//
// The wider four-plugin harness (buf generate + go build of the generated code)
// is documented in the delivery notes; this single-module test captures the
// same regression classes without cross-repo orchestration.
func TestIntegrationOutput(t *testing.T) {
	set := testutil.LoadDescriptorSet(t, "testdata/pb/integration.pb")
	plugin := testutil.MustCreatePlugin(t, set, "integration.proto")
	file := testutil.FileToGenerate(t, plugin)

	genFile, err := GenerateFile(plugin, file, DefaultConfig())
	if err != nil {
		t.Fatalf("GenerateFile failed: %v", err)
	}
	if genFile == nil {
		t.Fatal("expected a generated file, got nil")
	}
	raw, err := genFile.Content()
	if err != nil {
		t.Fatalf("Content failed: %v", err)
	}
	src := string(raw)

	// 1. Syntactic validity: the generated Go must parse.
	if _, err := goparser.ParseFile(token.NewFileSet(), "integration.sphere.pb.go", raw, goparser.AllErrors); err != nil {
		t.Fatalf("generated code is not valid Go: %v", err)
	}

	// 2. FORM binding must be consumed by the handler and advertised in Swagger.
	assertContains(t, src, "ctx.BindForm(&in)")
	assertContains(t, src, "// @Param filename formData string false \"filename\"")
	assertContains(t, src, "// @Param size formData integer false \"size\"")

	// 3. Well-known types as query params must render as scalar Swagger types
	//    (BUG-48), not the Go package type name.
	assertContains(t, src, "// @Param created_after query string false \"created_after\"") // Timestamp
	assertContains(t, src, "// @Param max_age query string false \"max_age\"")             // Duration
	assertContains(t, src, "// @Param keyword query string false \"keyword\"")             // StringValue
	assertContains(t, src, "// @Param limit query integer false \"limit\"")                // Int64Value
	assertContains(t, src, "// @Param active query boolean false \"active\"")              // BoolValue
	// The Go package identifiers of the well-known types must never reach the
	// output: before BUG-48 the Swagger param used QualifiedGoIdent (e.g.
	// timestamppb.Timestamp) which is not a valid Swagger type.
	for _, leaked := range []string{"timestamppb", "durationpb", "wrapperspb"} {
		if strings.Contains(src, leaked) {
			t.Errorf("well-known Go package %q leaked into the generated output", leaked)
		}
	}

	// 4. Each binding location wires up the matching Bind* call.
	assertContains(t, src, "ctx.BindJSON(&in)")   // JSON body (nested message + map + oneof)
	assertContains(t, src, "ctx.BindHeader(&in)") // header param
	assertContains(t, src, "ctx.BindQuery(&in)")  // query param
	assertContains(t, src, "ctx.BindURI(&in)")    // uri param
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("generated output missing expected fragment:\n  %q", needle)
	}
}
