package swagspec

import (
	"strings"
	"testing"

	swag "github.com/swaggo/swag"
)

func validOperation() *Operation {
	return &Operation{
		Summary:     "ListItems",
		Description: "List items for one tenant.",
		Tags:        []string{"v1", "v1.ItemService"},
		Accept:      "json",
		Produce:     "json",
		RawLines:    []string{"// @Security ApiKeyAuth"},
		Params: []Param{
			{Name: "tenant_id", In: LocationPath, Type: "string", Required: true, Description: "tenant_id"},
			{Name: "page", In: LocationQuery, Type: "integer", Required: false, Description: "page"},
			{Name: "names", In: LocationQuery, Type: "[]string", Required: false, Description: "names"},
			{Name: "request", In: LocationBody, Type: "string", Required: true, Description: "request body"},
		},
		Success:      Response{Codes: []string{"200"}, Type: "string"},
		Failure:      Response{Codes: []string{"400", "401", "403", "500", "default"}, Type: "string"},
		RouterPath:   "/v1/tenants/{tenant_id}/items",
		RouterMethod: "post",
	}
}

func TestRender(t *testing.T) {
	got, err := validOperation().Render()
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	want := strings.Join([]string{
		"// @Summary ListItems",
		"// @Description List items for one tenant.",
		"// @Tags v1,v1.ItemService",
		"// @Accept json",
		"// @Produce json",
		"// @Security ApiKeyAuth",
		`// @Param tenant_id path string true "tenant_id"`,
		`// @Param page query integer false "page"`,
		`// @Param names query []string false "names"`,
		`// @Param request body string true "request body"`,
		"// @Success 200 {object} string",
		"// @Failure 400,401,403,500,default {object} string",
		"// @Router /v1/tenants/{tenant_id}/items [post]",
	}, "\n")
	if got != want {
		t.Errorf("Render() mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestRenderStreamDescription(t *testing.T) {
	op := validOperation()
	op.Description = ""
	op.RawLines = nil
	op.Params = nil
	op.RouterPath = "/v1/stream"
	op.Success = Response{Codes: []string{"200"}, Type: "TailResponse", Description: "server-sent events stream of this message; terminated by a done or error event"}
	got, err := op.Render()
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(got, `// @Success 200 {object} TailResponse "server-sent events stream of this message; terminated by a done or error event"`) {
		t.Errorf("stream description not rendered: %s", got)
	}
}

func TestValidateRejectsInvalidOperations(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Operation)
		wantErr string
	}{
		{"empty summary", func(o *Operation) { o.Summary = "" }, "@Summary is required"},
		{"multiline summary", func(o *Operation) { o.Summary = "a\nb" }, "must be a single line"},
		{"router path without slash", func(o *Operation) { o.RouterPath = "v1/items" }, `must start with /`},
		{"router path whitespace", func(o *Operation) { o.RouterPath = "/v1/items extra" }, "whitespace"},
		{"bad router method", func(o *Operation) { o.RouterMethod = "GET ALL" }, "valid verb"},
		{"empty tag", func(o *Operation) { o.Tags = []string{"", "svc"} }, "empty"},
		{"raw line without comment prefix", func(o *Operation) { o.RawLines = []string{"@Security x"} }, `must start with //`},
		{"empty response type", func(o *Operation) { o.Success.Type = "" }, "require a type"},
		{"bad response code", func(o *Operation) { o.Failure.Codes = []string{"4xx"} }, "numeric"},
		{"no response code", func(o *Operation) { o.Failure.Codes = nil }, "at least one status code"},

		{"path param missing from route", func(o *Operation) { o.Params[0].Name = "other" }, "is not present in @Router path"},
		{"optional path param", func(o *Operation) { o.Params[0].Required = false }, "must be required"},
		{"array path param", func(o *Operation) { o.Params[0].Type = "[]string" }, "array type"},
		{"map path param", func(o *Operation) { o.Params[0].Type = "map[string]string" }, "map type"},

		{"map query param", func(o *Operation) { o.Params[1].Type = "map[string]string" }, "map types"},
		{"array of refs in query", func(o *Operation) { o.Params[1].Type = "[]v1.Item" }, "arrays of primitives"},
		{"quoted param description", func(o *Operation) { o.Params[2].Description = `say "hi"` }, "double quotes"},

		{"formData on get", func(o *Operation) {
			o.Params = o.Params[:1]
			o.Params[0] = Param{Name: "upload", In: LocationFormData, Type: "string", Description: "upload"}
			o.RouterMethod = "get"
		}, "not allowed on [get]"},
		{"body param on get", func(o *Operation) {
			o.Params = o.Params[:1]
			o.Params[0] = Param{Name: "request", In: LocationBody, Type: "string", Required: true, Description: "request body"}
			o.RouterMethod = "get"
		}, "not allowed on [get]"},
		{"duplicate param", func(o *Operation) {
			o.Params = append(o.Params, Param{Name: "page", In: LocationQuery, Type: "integer", Description: "page"})
		}, "duplicate"},
		{"path variable without param", func(o *Operation) { o.RouterPath = "/v1/tenants/{tenant_id}/items/{item_id}" }, "{item_id}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := validOperation()
			tt.mutate(op)
			err := op.Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %q, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestRoundTripWithSwagParser feeds a rendered block back through swag's own
// annotation parser and asserts the parse result. This is the synthesizer's
// oracle: whatever the renderer emits must survive swag's real grammar. The
// block uses Go-primitive types only because resolving referenced types would
// require scanning real Go source, which the golden/fixture tests already
// cover end to end.
func TestRoundTripWithSwagParser(t *testing.T) {
	op := validOperation()
	block, err := op.Render()
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	parsed := swag.NewOperation(swag.New())
	for _, line := range strings.Split(block, "\n") {
		if err := parsed.ParseComment(line, nil); err != nil {
			t.Fatalf("swag rejected rendered line %q: %v", line, err)
		}
	}

	if parsed.Summary != "ListItems" {
		t.Errorf("parsed summary = %q, want %q", parsed.Summary, "ListItems")
	}
	if len(parsed.Parameters) != len(op.Params) {
		t.Fatalf("parsed %d parameters, want %d", len(parsed.Parameters), len(op.Params))
	}
	for i, want := range op.Params {
		got := parsed.Parameters[i].ParamProps
		if got.Name != want.Name {
			t.Errorf("param %d name = %q, want %q", i, got.Name, want.Name)
		}
		if string(got.In) != string(want.In) {
			t.Errorf("param %s in = %q, want %q", want.Name, got.In, want.In)
		}
		if got.Required != want.Required {
			t.Errorf("param %s required = %v, want %v", want.Name, got.Required, want.Required)
		}
		if want.In == LocationBody && (got.Schema == nil || got.Schema.Type == nil) {
			t.Errorf("param %s body schema missing", want.Name)
		}
	}
	// swag's parser splits the comma-separated failure codes into individual
	// status responses plus the "default" response; @Success 200 adds one.
	if len(parsed.Responses.StatusCodeResponses) != 5 {
		t.Fatalf("parsed %d status responses, want 5 (200,400,401,403,500)", len(parsed.Responses.StatusCodeResponses))
	}
	for _, code := range []int{200, 400, 401, 403, 500} {
		if _, ok := parsed.Responses.StatusCodeResponses[code]; !ok {
			t.Errorf("parsed responses missing %d", code)
		}
	}
	if parsed.Responses.Default == nil {
		t.Errorf("parsed default response missing")
	}
	if len(parsed.RouterProperties) != 1 ||
		parsed.RouterProperties[0].Path != "/v1/tenants/{tenant_id}/items" ||
		!strings.EqualFold(parsed.RouterProperties[0].HTTPMethod, "post") {
		t.Errorf("parsed router = %+v, want one /v1/tenants/{tenant_id}/items [post]", parsed.RouterProperties)
	}
}
