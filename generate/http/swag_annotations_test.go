package http

import (
	"strings"
	"testing"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/testutil"
)

// swagAnnotations generates the swag_edge.proto fixture and splits the output
// into per-handler swag blocks keyed by handler function name.
func swagAnnotations(t *testing.T) map[string]string {
	t.Helper()
	set := testutil.LoadDescriptorSet(t, "testdata/pb/swag_edge.pb")
	plugin := testutil.MustCreatePlugin(t, set, "swag_edge.proto")
	file := testutil.FileToGenerate(t, plugin)
	genFile, err := GenerateFile(plugin, file, DefaultConfig())
	if err != nil {
		t.Fatalf("GenerateFile failed: %v", err)
	}
	content, err := genFile.Content()
	if err != nil {
		t.Fatalf("Content failed: %v", err)
	}
	return splitSwagBlocks(string(content))
}

// splitSwagBlocks maps "func _Svc_Method0_HTTP_Handler" style signatures to the
// comment block that immediately precedes them.
func splitSwagBlocks(content string) map[string]string {
	blocks := make(map[string]string)
	lines := strings.Split(content, "\n")
	var comment []string
	for _, line := range lines {
		if strings.HasPrefix(line, "// @") {
			comment = append(comment, line)
			continue
		}
		if handler, ok := extractHandlerName(line); ok && len(comment) > 0 {
			blocks[handler] = strings.Join(comment, "\n")
		}
		if !strings.HasPrefix(line, "//") {
			comment = comment[:0]
		}
	}
	return blocks
}

func extractHandlerName(line string) (string, bool) {
	if !strings.HasPrefix(line, "func _") {
		return "", false
	}
	name := line[len("func "):]
	if idx := strings.Index(name, "("); idx >= 0 {
		name = name[:idx]
	}
	return name, true
}

// assertAnnotations checks that the block for handler contains every want line.
func assertAnnotations(t *testing.T, blocks map[string]string, handler string, want []string) {
	t.Helper()
	block, ok := blocks[handler]
	if !ok {
		t.Fatalf("no swag block found for %s; got blocks: %v", handler, func() []string {
			keys := make([]string, 0, len(blocks))
			for k := range blocks {
				keys = append(keys, k)
			}
			return keys
		}())
	}
	for _, line := range want {
		if !strings.Contains(block+"\n", line+"\n") {
			t.Errorf("%s: missing annotation line %q\nblock:\n%s", handler, line, block)
		}
	}
}

// TestSwagAnnotationEdgeCases pins the Swagger annotation output for the edge
// cases in testdata/proto/swag_edge.proto. Each expectation reflects the
// CURRENT generator behavior; entries marked BUG document known mismatches that
// should flip to corrected expectations once fixed.
func TestSwagAnnotationEdgeCases(t *testing.T) {
	blocks := swagAnnotations(t)

	t.Run("no_body_post_omits_body_param", func(t *testing.T) {
		// A POST without a declared body only reads query params, so the
		// annotation must not advertise a request body (fixed by routing the
		// body param through swagspec with HasBody).
		block, ok := blocks["_SwagEdgeService_NoBodyPost0_HTTP_Handler"]
		if !ok {
			t.Fatal("no swag block for NoBodyPost handler")
		}
		if strings.Contains(block, "@Param request body") {
			t.Errorf("no-body POST must not document a request body:\n%s", block)
		}
		assertAnnotations(t, blocks, "_SwagEdgeService_NoBodyPost0_HTTP_Handler", []string{
			"// @Accept json",
			"// @Param name query string false \"name\"",
			"// @Success 200 {object} httpz.DataResponse[NoBodyPostResponse]",
			"// @Failure 400,401,403,500,default {object} httpz.ErrorResponse",
			"// @Router /api/swag/post [post]",
		})
	})

	t.Run("get_with_form_documents_query_params", func(t *testing.T) {
		// Form-bound fields on a no-body method are read from the query
		// string at runtime (gin form binding), so they must be documented
		// as query parameters, and OpenAPI 2.0 forbids formData on GET.
		assertAnnotations(t, blocks, "_SwagEdgeService_GetWithForm0_HTTP_Handler", []string{
			"// @Accept json",
			"// @Param upload_name query string false \"upload_name\"",
			"// @Param upload_size query integer false \"upload_size\"",
			"// @Router /api/swag/form [get]",
		})
		block, ok := blocks["_SwagEdgeService_GetWithForm0_HTTP_Handler"]
		if !ok {
			t.Fatal("no swag block for GetWithForm handler")
		}
		if strings.Contains(block, "formData") || strings.Contains(block, "mpfd") {
			t.Errorf("GET must not carry formData parameters:\n%s", block)
		}
	})

	t.Run("repeated_wildcard_path_param_is_single_token", func(t *testing.T) {
		// A repeated string bound to a {name=**} catch-all still arrives as
		// ONE raw slash-separated token, so the path parameter must be a
		// scalar, not an unbindable array.
		assertAnnotations(t, blocks, "_SwagEdgeService_WildcardList0_HTTP_Handler", []string{
			"// @Param names path string true \"names\"",
			"// @Router /api/swag/repeated/{names} [get]",
		})
	})

	t.Run("map_body_projects_onto_map_schema", func(t *testing.T) {
		// OK: swag resolves map[string]string into an additionalProperties
		// schema (verified against swag v1.16.6).
		assertAnnotations(t, blocks, "_SwagEdgeService_MapBody0_HTTP_Handler", []string{
			"// @Param request body map[string]string true \"request body\"",
			"// @Router /api/swag/map [post]",
		})
	})

	t.Run("scalar_response_body", func(t *testing.T) {
		// OK for docs: DataResponse[string] is a valid swag generic and
		// resolves to {data: {type: string}} (verified against swag v1.16.6).
		//
		// BUG in the handler body (not the annotation): the response type
		// becomes a value type (string), but the template always emits
		// `return nil, err`, which does not compile for (string, error).
		// See TestSwagScalarResponseBodyHandlerDoesNotCompile.
		assertAnnotations(t, blocks, "_SwagEdgeService_ScalarResponseBody0_HTTP_Handler", []string{
			"// @Success 200 {object} httpz.DataResponse[string]",
			"// @Router /api/swag/scalar-response [get]",
		})
	})

	t.Run("map_response_body", func(t *testing.T) {
		assertAnnotations(t, blocks, "_SwagEdgeService_MapResponseBody0_HTTP_Handler", []string{
			"// @Success 200 {object} httpz.DataResponse[map[string]string]",
			"// @Router /api/swag/map-response [get]",
		})
	})

	t.Run("list_response_body", func(t *testing.T) {
		assertAnnotations(t, blocks, "_SwagEdgeService_ListResponseBody0_HTTP_Handler", []string{
			"// @Success 200 {object} httpz.DataResponse[[]EdgeItem]",
			"// @Router /api/swag/list-response [get]",
		})
	})

	t.Run("enum_query_param", func(t *testing.T) {
		// OK: enum-bound query params render the Go enum type; swag v1.16.6
		// resolves it and emits the enum values.
		assertAnnotations(t, blocks, "_SwagEdgeService_EnumQuery0_HTTP_Handler", []string{
			"// @Param status query EdgeStatus false \"status\"",
			"// @Router /api/swag/enum [get]",
		})
	})

	t.Run("enum_list_query_param_renders_primitive_array", func(t *testing.T) {
		// swag rejects arrays of non-primitive elements in parameters
		// ("X is not supported array type for query"), so repeated enums
		// collapse to their underlying scalar.
		assertAnnotations(t, blocks, "_SwagEdgeService_EnumListQuery0_HTTP_Handler", []string{
			"// @Param statuses query []integer false \"statuses\"",
			"// @Router /api/swag/enum-list [get]",
		})
	})
}

// TestSwagNoBodyPostHandlerSkipsBindJSON guards the runtime side of the
// no-body POST: the handler must not attempt to decode a request body.
func TestSwagNoBodyPostHandlerSkipsBindJSON(t *testing.T) {
	set := testutil.LoadDescriptorSet(t, "testdata/pb/swag_edge.pb")
	plugin := testutil.MustCreatePlugin(t, set, "swag_edge.proto")
	file := testutil.FileToGenerate(t, plugin)
	genFile, err := GenerateFile(plugin, file, DefaultConfig())
	if err != nil {
		t.Fatalf("GenerateFile failed: %v", err)
	}
	content, err := genFile.Content()
	if err != nil {
		t.Fatalf("Content failed: %v", err)
	}
	handler := extractFuncBody(t, string(content), "_SwagEdgeService_NoBodyPost0_HTTP_Handler")
	if strings.Contains(handler, "BindJSON") {
		t.Errorf("no-body POST handler must not bind JSON:\n%s", handler)
	}
	if !strings.Contains(handler, "BindQuery") {
		t.Errorf("no-body POST handler should bind query params:\n%s", handler)
	}
}

// TestSwagScalarResponseBodyHandlerDoesNotCompile pins the scalar response_body
// compile bug: BUG: with response_body projected onto a scalar field the
// handler returns a value type (e.g. (string, error)), but the template
// unconditionally emits `return nil, err`, which is a compile error for
// non-pointer, non-map, non-slice types. This test documents the current
// (broken) emission; when fixed, it should assert `return "", err` (or an
// equivalent zero value) instead.
func TestSwagScalarResponseBodyHandlerDoesNotCompile(t *testing.T) {
	set := testutil.LoadDescriptorSet(t, "testdata/pb/swag_edge.pb")
	plugin := testutil.MustCreatePlugin(t, set, "swag_edge.proto")
	file := testutil.FileToGenerate(t, plugin)
	genFile, err := GenerateFile(plugin, file, DefaultConfig())
	if err != nil {
		t.Fatalf("GenerateFile failed: %v", err)
	}
	content, err := genFile.Content()
	if err != nil {
		t.Fatalf("Content failed: %v", err)
	}
	handler := extractFuncBody(t, string(content), "_SwagEdgeService_ScalarResponseBody0_HTTP_Handler")
	if !strings.Contains(handler, "func(ctx httpx.Context) (string, error)") {
		t.Fatalf("expected scalar (string, error) handler signature:\n%s", handler)
	}
	if !strings.Contains(handler, "return nil, err") {
		t.Errorf("BUG marker lost: the scalar response_body handler no longer emits `return nil, err` (was a compile error). Update this test to the corrected zero-value returns.")
	}
}

// TestCustomVerbPathGenerationFails pins the custom-verb path limitation:
// BUG: a google.api.http custom-method path such as `/v1/reports:generate` is
// rejected outright because the `:generate` suffix is mistaken for a gin-style
// path parameter. Expected fix: treat a trailing `:verb` literal segment as a
// literal instead of a parameter.
func TestCustomVerbPathGenerationFails(t *testing.T) {
	set := testutil.LoadDescriptorSet(t, "testdata/pb/custom_verb.pb")
	plugin := testutil.MustCreatePlugin(t, set, "custom_verb.proto")
	file := testutil.FileToGenerate(t, plugin)
	_, err := GenerateFile(plugin, file, DefaultConfig())
	if err == nil {
		t.Fatal("BUG marker lost: custom verb paths now generate successfully; update this test and parser.HTTPRouteToSwaggerRoute expectations")
	}
	if !strings.Contains(err.Error(), "does not match a top-level request field") {
		t.Fatalf("unexpected error for custom verb path: %v", err)
	}
}

func extractFuncBody(t *testing.T, content, handlerName string) string {
	t.Helper()
	start := strings.Index(content, "func "+handlerName)
	if start < 0 {
		t.Fatalf("handler %s not found", handlerName)
	}
	end := strings.Index(content[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("handler %s body not terminated", handlerName)
	}
	return content[start : start+end]
}
