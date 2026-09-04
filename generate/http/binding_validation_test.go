package http

import (
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
			wantErr: "method `InvalidService.QueryMessage` field `inner` of type `message` cannot be bound to QUERY: only scalar types (and well-known scalar wrappers) are supported there. File: `invalid_binding.proto`, Message: `QueryMessageRequest`",
		},
		{
			name:   "message bound to URI",
			method: "UriMessage",
			invoke: func(method *protogen.Method) error {
				_, err := parser.URIParams(method, "/api/u/:inner")
				return err
			},
			wantErr: "method `InvalidService.UriMessage` field `inner` of type `message` cannot be bound to URI: only scalar types (and well-known scalar wrappers) are supported there. File: `invalid_binding.proto`, Message: `UriMessageRequest`",
		},
		{
			name:   "map bound to HEADER",
			method: "HeaderMap",
			invoke: func(method *protogen.Method) error {
				_, err := parser.HeaderParams(method)
				return err
			},
			wantErr: "method `InvalidService.HeaderMap` field `data` of type `map` cannot be bound to HEADER: only scalar types (and well-known scalar wrappers) are supported there. File: `invalid_binding.proto`, Message: `HeaderMapRequest`",
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
