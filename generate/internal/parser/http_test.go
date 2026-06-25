package parser

import (
	"net/http"
	"testing"

	"google.golang.org/genproto/googleapis/api/annotations"
)

func TestParseHttpRule(t *testing.T) {
	tests := []struct {
		name             string
		rule             *annotations.HttpRule
		wantPath         string
		wantMethod       string
		wantHasBody      bool
		wantBody         string
		wantResponseBody string
	}{
		{
			name:       "get",
			rule:       &annotations.HttpRule{Pattern: &annotations.HttpRule_Get{Get: "/a"}},
			wantPath:   "/a",
			wantMethod: http.MethodGet,
		},
		{
			name:       "put",
			rule:       &annotations.HttpRule{Pattern: &annotations.HttpRule_Put{Put: "/a"}},
			wantPath:   "/a",
			wantMethod: http.MethodPut,
		},
		{
			name:        "post body star is normalized to empty",
			rule:        &annotations.HttpRule{Pattern: &annotations.HttpRule_Post{Post: "/a"}, Body: "*"},
			wantPath:    "/a",
			wantMethod:  http.MethodPost,
			wantHasBody: true,
			wantBody:    "",
		},
		{
			name:        "post named body",
			rule:        &annotations.HttpRule{Pattern: &annotations.HttpRule_Post{Post: "/a"}, Body: "payload"},
			wantPath:    "/a",
			wantMethod:  http.MethodPost,
			wantHasBody: true,
			wantBody:    "payload",
		},
		{
			name:        "post no body",
			rule:        &annotations.HttpRule{Pattern: &annotations.HttpRule_Post{Post: "/a"}},
			wantPath:    "/a",
			wantMethod:  http.MethodPost,
			wantHasBody: false,
		},
		{
			name:       "delete",
			rule:       &annotations.HttpRule{Pattern: &annotations.HttpRule_Delete{Delete: "/a"}},
			wantPath:   "/a",
			wantMethod: http.MethodDelete,
		},
		{
			name:       "patch",
			rule:       &annotations.HttpRule{Pattern: &annotations.HttpRule_Patch{Patch: "/a"}},
			wantPath:   "/a",
			wantMethod: http.MethodPatch,
		},
		{
			name: "custom",
			rule: &annotations.HttpRule{Pattern: &annotations.HttpRule_Custom{
				Custom: &annotations.CustomHttpPattern{Kind: "OPTIONS", Path: "/a"},
			}},
			wantPath:   "/a",
			wantMethod: "OPTIONS",
		},
		{
			name:             "response body star is normalized to empty",
			rule:             &annotations.HttpRule{Pattern: &annotations.HttpRule_Get{Get: "/a"}, ResponseBody: "*"},
			wantPath:         "/a",
			wantMethod:       http.MethodGet,
			wantResponseBody: "",
		},
		{
			name:             "response body named",
			rule:             &annotations.HttpRule{Pattern: &annotations.HttpRule_Get{Get: "/a"}, ResponseBody: "data"},
			wantPath:         "/a",
			wantMethod:       http.MethodGet,
			wantResponseBody: "data",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseHttpRule(tt.rule)
			if got.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", got.Path, tt.wantPath)
			}
			if got.Method != tt.wantMethod {
				t.Errorf("Method = %q, want %q", got.Method, tt.wantMethod)
			}
			if got.HasBody != tt.wantHasBody {
				t.Errorf("HasBody = %v, want %v", got.HasBody, tt.wantHasBody)
			}
			if got.Body != tt.wantBody {
				t.Errorf("Body = %q, want %q", got.Body, tt.wantBody)
			}
			if got.ResponseBody != tt.wantResponseBody {
				t.Errorf("ResponseBody = %q, want %q", got.ResponseBody, tt.wantResponseBody)
			}
		})
	}
}
