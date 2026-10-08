package http

import (
	"testing"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/parser"
	"github.com/go-sphere/protoc-gen-sphere/generate/internal/testutil"
	"google.golang.org/protobuf/compiler/protogen"
)

// TestBindingLocationKindValidation verifies that requests the generated
// handler cannot bind are rejected at generation time instead of silently
// emitting a binding the runtime cannot satisfy: fields whose type has no
// single-token form (map / message / bytes / well-known types) bound to QUERY /
// URI / HEADER, FORM fields on GET, real oneofs in the request, and body /
// response_body selectors that are not top-level fields.
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

	tests := []struct {
		name    string
		method  string
		invoke  func(*protogen.Method) error
		wantErr string
	}{
		{
			name:   "message bound to QUERY",
			method: "QueryMessage",
			invoke: func(method *protogen.Method) error {
				_, err := parser.QueryParams(method, "POST", nil)
				return err
			},
			wantErr: "method `InvalidService.QueryMessage` field `inner` of type `message` cannot be bound to QUERY: only scalar and enum types are supported there. File: `invalid_binding.proto`, Message: `QueryMessageRequest`",
		},
		{
			name:   "message bound to URI",
			method: "UriMessage",
			invoke: func(method *protogen.Method) error {
				_, err := parser.URIParams(method, "/api/u/:inner")
				return err
			},
			wantErr: "method `InvalidService.UriMessage` field `inner` of type `message` cannot be bound to URI: only scalar and enum types are supported there. File: `invalid_binding.proto`, Message: `UriMessageRequest`",
		},
		{
			name:   "map bound to HEADER",
			method: "HeaderMap",
			invoke: func(method *protogen.Method) error {
				_, err := parser.HeaderParams(method)
				return err
			},
			wantErr: "method `InvalidService.HeaderMap` field `data` of type `map` cannot be bound to HEADER: only scalar and enum types are supported there. File: `invalid_binding.proto`, Message: `HeaderMapRequest`",
		},
		{
			name:   "well-known type bound to QUERY",
			method: "QueryTimestamp",
			invoke: func(method *protogen.Method) error {
				_, err := parser.QueryParams(method, "GET", nil)
				return err
			},
			wantErr: "method `InvalidService.QueryTimestamp` field `since` of type `message` cannot be bound to QUERY: only scalar and enum types are supported there. File: `invalid_binding.proto`, Message: `QueryTimestampRequest`",
		},
		{
			name:   "FORM on GET",
			method: "FormOnGet",
			invoke: func(method *protogen.Method) error {
				_, err := parser.QueryParams(method, "GET", nil)
				return err
			},
			wantErr: "method `InvalidService.FormOnGet` field `name` is bound to FORM, which is not allowed on GET: the request has no body, and not every httpx adapter reads form values from the query string; bind it to QUERY instead. File: `invalid_binding.proto`, Field: `FormOnGetRequest`",
		},
		{
			name:    "nested oneof in request",
			method:  "NestedOneof",
			invoke:  checkRequestOneof,
			wantErr: "method `InvalidService.NestedOneof` request field `item` contains oneof `kind` (message `testdata.invalid.v1.WithOneof`), which generated HTTP handlers cannot bind: the JSON body is decoded with encoding/json, which ignores oneof fields, and query/uri/header/form binding skips them; replace the oneof with separate optional fields. File: `invalid_binding.proto`, Message: `NestedOneofRequest`",
		},
		{
			name:   "nested body path",
			method: "NestedBody",
			invoke: func(method *protogen.Method) error {
				return checkTopLevelField(method, method.Input, "body", "inner.a")
			},
			wantErr: "method `InvalidService.NestedBody` body `inner.a` is a nested field path; google.api.http requires a top-level field of message `BodyFieldRequest`. File: `invalid_binding.proto`",
		},
		{
			name:   "unknown body field",
			method: "UnknownBody",
			invoke: func(method *protogen.Method) error {
				return checkTopLevelField(method, method.Input, "body", "missing")
			},
			wantErr: "method `InvalidService.UnknownBody` body `missing` is not a field of message `BodyFieldRequest`. File: `invalid_binding.proto`",
		},
		{
			name:   "nested response_body path",
			method: "NestedResponseBody",
			invoke: func(method *protogen.Method) error {
				return checkTopLevelField(method, method.Output, "response_body", "inner.a")
			},
			wantErr: "method `InvalidService.NestedResponseBody` response_body `inner.a` is a nested field path; google.api.http requires a top-level field of message `NestedResponse`. File: `invalid_binding.proto`",
		},
		{
			// A warning, promoted to an error under fail_on_warn. Only the
			// JSON-located sibling is reported; the HEADER field has a source.
			name:   "JSON field outside body under fail_on_warn",
			method: "FieldsOutsideBody",
			invoke: func(method *protogen.Method) error {
				return warnFieldsOutsideBody(method, "inner", &fileConfig{failOnWarn: true})
			},
			wantErr: "method `InvalidService.FieldsOutsideBody` field `version` is bound to the JSON body, but body is `inner`, so the field is never bound; give it a QUERY, URI or HEADER location, or use body: \"*\". File: `invalid_binding.proto`, Message: `BodyFieldRequest`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := methods[tt.method]
			if method == nil {
				t.Fatalf("method %s not found", tt.method)
			}
			err := tt.invoke(method)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tt.method)
			}
			if got := err.Error(); got != tt.wantErr {
				t.Errorf("error = %q, want %q", got, tt.wantErr)
			}
		})
	}

	t.Run("whole file generation fails fast", func(t *testing.T) {
		// GenerateFile must surface the first binding error rather than emitting a
		// partial file.
		if _, err := GenerateFile(plugin, file, DefaultConfig()); err == nil {
			t.Fatal("expected GenerateFile to fail on invalid binding, got nil")
		}
	})
}

func TestQueryParamsGETAllowsHeader(t *testing.T) {
	set := testutil.LoadDescriptorSet(t, "testdata/pb/no_body.pb")
	plugin := testutil.MustCreatePlugin(t, set, "no_body.proto")
	file := testutil.FileToGenerate(t, plugin)
	var getItem *protogen.Method
	for _, svc := range file.Services {
		for _, m := range svc.Methods {
			if m.GoName == "GetItem" {
				getItem = m
			}
		}
	}
	if getItem == nil {
		t.Fatal("GetItem not found")
	}
	vars, err := parser.URIParams(getItem, "/api/items/:id")
	if err != nil {
		t.Fatalf("URIParams: %v", err)
	}
	queries, err := parser.QueryParams(getItem, "GET", vars)
	if err != nil {
		t.Fatalf("GET with HEADER field must not fail QueryParams: %v", err)
	}
	if len(queries) != 1 || queries[0].Name != "filter" {
		t.Fatalf("queries = %+v, want filter only", queries)
	}
	headers, err := parser.HeaderParams(getItem)
	if err != nil {
		t.Fatalf("HeaderParams: %v", err)
	}
	if len(headers) != 1 || headers[0].Name != "auth_token" {
		t.Fatalf("headers = %+v, want auth_token", headers)
	}
}

func TestURIParamsUnmatchedRouteParam(t *testing.T) {
	set := testutil.LoadDescriptorSet(t, "testdata/pb/no_body.pb")
	plugin := testutil.MustCreatePlugin(t, set, "no_body.proto")
	file := testutil.FileToGenerate(t, plugin)
	var getItem *protogen.Method
	for _, svc := range file.Services {
		for _, m := range svc.Methods {
			if m.GoName == "GetItem" {
				getItem = m
			}
		}
	}
	if getItem == nil {
		t.Fatal("GetItem not found")
	}
	_, err := parser.URIParams(getItem, "/v1/users/:user_id")
	if err == nil {
		t.Fatal("expected error for unmatched {user.id}/:user_id route param")
	}
	want := "method `NoBodyService.GetItem` route `/v1/users/:user_id` has parameter `user_id` that does not match a top-level request field. Nested path variables such as {user.id} are not supported; declare a top-level field and mark it BINDING_LOCATION_URI. File: `no_body.proto`, Message: `GetItemRequest`"
	if got := err.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}
