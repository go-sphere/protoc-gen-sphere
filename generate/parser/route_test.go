package parser

import "testing"

func TestHTTPRoute(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"simple param", "/api/test/{id}", "/api/test/:id", false},
		{"two params", "/api/test/{a}/second/{b}", "/api/test/:a/second/:b", false},
		{"single wildcard", "/api/{name=*}", "/api/:name", false},
		{"double wildcard", "/api/{path=**}", "/api/*path", false},
		{"literal", "/api/{ver=v1}", "/api/v1", false},
		{"literal single wildcard", "/files/{path=assets/*}", "/files/assets/:path", false},
		{"literal double wildcard", "/files/{path=assets/**}", "/files/assets/*path", false},
		{"no params", "/api/test", "/api/test", false},
		{"adds leading slash", "api/test", "/api/test", false},
		{"dotted param name cleaned", "/api/{a.b}", "/api/:a_b", false},
		{"trailing slash trimmed", "/api/test/", "/api/test", false},
		{"empty is error", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HTTPRoute(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("HTTPRoute(%q) error = %v, wantErr = %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("HTTPRoute(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHTTPRouteToSwaggerRoute(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/api/test/:id", "/api/test/{id}"},
		{"/api/test/:a/second/:b", "/api/test/{a}/second/{b}"},
		{"/files/*path", "/files/{path}"},
		{"/api/test", "/api/test"},
	}
	for _, tt := range tests {
		if got := HTTPRouteToSwaggerRoute(tt.in); got != tt.want {
			t.Errorf("HTTPRouteToSwaggerRoute(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCleanParamName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"abc", "abc"},
		{"a.b", "a_b"},
		{"a-b!", "a_b_"},
		{"a_b", "a_b"},
	}
	for _, tt := range tests {
		if got := cleanParamName(tt.in); got != tt.want {
			t.Errorf("cleanParamName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
