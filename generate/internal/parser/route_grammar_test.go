package parser

import (
	"strings"
	"testing"
)

func kinds(vs []RouteViolation) []RouteViolationKind {
	out := make([]RouteViolationKind, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Kind)
	}
	return out
}

func sameKinds(got, want []RouteViolationKind) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestRouteViolationsFromProtoPath walks the full pipeline: a google.api.http
// path through HTTPRoute, then the converted route through the grammar check.
// Both halves are pinned, because a change in the conversion is what decides
// which shapes the check ever sees.
func TestRouteViolationsFromProtoPath(t *testing.T) {
	tests := []struct {
		name      string
		protoPath string
		wantRoute string
		wantKinds []RouteViolationKind
	}{
		{
			name:      "colon after static segment",
			protoPath: "/v1/reports:generate",
			wantRoute: "/v1/reports:generate",
			wantKinds: []RouteViolationKind{RouteViolationColonInStatic},
		},
		{
			name:      "colon after parameter",
			protoPath: "/v1/users/{id}:activate",
			wantRoute: "/v1/users/:id:activate",
			wantKinds: []RouteViolationKind{RouteViolationColonAfterParam},
		},
		{
			name:      "colon after parameter from a literal prefix pattern",
			protoPath: "/v1/{name=projects/*}:cancel",
			wantRoute: "/v1/projects/:name:cancel",
			wantKinds: []RouteViolationKind{RouteViolationColonAfterParam},
		},
		{
			name:      "colon after wildcard",
			protoPath: "/v1/files/{path=**}:archive",
			wantRoute: "/v1/files/*path:archive",
			wantKinds: []RouteViolationKind{RouteViolationColonAfterWildcard},
		},
		{
			name:      "wildcard is not the final segment",
			protoPath: "/v1/{path=**}/more",
			wantRoute: "/v1/*path/more",
			wantKinds: []RouteViolationKind{RouteViolationWildcardNotFinal},
		},
		{
			name:      "two wildcards",
			protoPath: "/v1/{a=**}/x/{b=**}",
			wantRoute: "/v1/*a/x/*b",
			wantKinds: []RouteViolationKind{RouteViolationWildcardNotFinal, RouteViolationExtraWildcard},
		},
		{
			name:      "plain parameter is inside the grammar",
			protoPath: "/v1/items/{id}",
			wantRoute: "/v1/items/:id",
		},
		{
			name:      "final wildcard is inside the grammar",
			protoPath: "/v1/files/{path=**}",
			wantRoute: "/v1/files/*path",
		},
		{
			name:      "static path is inside the grammar",
			protoPath: "/v1/items",
			wantRoute: "/v1/items",
		},
		{
			name:      "literal prefix with a final wildcard is inside the grammar",
			protoPath: "/v1/archive/{path=assets/**}",
			wantRoute: "/v1/archive/assets/*path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, err := HTTPRoute(tt.protoPath)
			if err != nil {
				t.Fatalf("HTTPRoute(%q) error = %v", tt.protoPath, err)
			}
			if route != tt.wantRoute {
				t.Fatalf("HTTPRoute(%q) = %q, want %q", tt.protoPath, route, tt.wantRoute)
			}
			got := RouteViolations(route)
			if !sameKinds(kinds(got), tt.wantKinds) {
				t.Fatalf("RouteViolations(%q) kinds = %v, want %v", route, kinds(got), tt.wantKinds)
			}
		})
	}
}

// TestRouteViolationsShapes covers the shapes that reach RouteViolations from a
// literal path written in the proto rather than from a {param} substitution.
func TestRouteViolationsShapes(t *testing.T) {
	tests := []struct {
		name      string
		route     string
		wantKinds []RouteViolationKind
	}{
		{name: "root", route: "/"},
		{name: "unnamed parameter", route: "/v1/:", wantKinds: []RouteViolationKind{RouteViolationUnnamedParam}},
		{name: "doubled colon parameter", route: "/v1/::x", wantKinds: []RouteViolationKind{RouteViolationUnnamedParam}},
		{name: "unnamed wildcard", route: "/v1/files/*", wantKinds: []RouteViolationKind{RouteViolationUnnamedWildcard}},
		{name: "wildcard mid segment", route: "/v1/fo*o", wantKinds: []RouteViolationKind{RouteViolationWildcardMidSegment}},
		{name: "star inside a parameter segment", route: "/v1/:id*x", wantKinds: []RouteViolationKind{RouteViolationWildcardMidSegment}},
		{name: "two wildcards in one segment", route: "/v1/*a*b", wantKinds: []RouteViolationKind{RouteViolationExtraWildcard}},
		{
			name:      "unnamed wildcard that is also not final",
			route:     "/v1/*/more",
			wantKinds: []RouteViolationKind{RouteViolationWildcardNotFinal, RouteViolationUnnamedWildcard},
		},
		{
			name:      "colon in a static segment before a parameter",
			route:     "/v1/reports:generate/:id",
			wantKinds: []RouteViolationKind{RouteViolationColonInStatic},
		},
		{
			// One warning per kind, however many segments repeat the mistake.
			name:      "repeated colon segments report once",
			route:     "/v1/a:b/c:d/e:f",
			wantKinds: []RouteViolationKind{RouteViolationColonInStatic},
		},
		{
			name:      "a colon and a wildcard violation are reported separately",
			route:     "/v1/reports:generate/*path/more",
			wantKinds: []RouteViolationKind{RouteViolationColonInStatic, RouteViolationWildcardNotFinal},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RouteViolations(tt.route)
			if !sameKinds(kinds(got), tt.wantKinds) {
				t.Fatalf("RouteViolations(%q) kinds = %v, want %v", tt.route, kinds(got), tt.wantKinds)
			}
		})
	}
}

// TestRouteViolationDetails checks that each reported violation carries the
// offending segment and a usable message. The exact wording is not pinned —
// the warning is prose for a human — but a detail that names no segment or
// says nothing about the consequence is a defect.
func TestRouteViolationDetails(t *testing.T) {
	routes := []string{
		"/v1/reports:generate",
		"/v1/users/:id:activate",
		"/v1/files/*path:archive",
		"/v1/*path/more",
		"/v1/*a/x/*b",
		"/v1/files/*",
		"/v1/:",
		"/v1/fo*o",
	}
	for _, route := range routes {
		got := RouteViolations(route)
		if len(got) == 0 {
			t.Errorf("RouteViolations(%q) = none, want at least one violation", route)
			continue
		}
		for _, v := range got {
			if v.Segment == "" {
				t.Errorf("RouteViolations(%q): violation %v has no segment", route, v.Kind)
			}
			if !strings.Contains(route, v.Segment) {
				t.Errorf("RouteViolations(%q): segment %q is not part of the route", route, v.Segment)
			}
			if !strings.Contains(v.Detail, v.Segment) {
				t.Errorf("RouteViolations(%q): detail %q does not name segment %q", route, v.Detail, v.Segment)
			}
			if strings.HasPrefix(v.Kind.String(), "unknown(") {
				t.Errorf("RouteViolations(%q): violation has an unnamed kind %d", route, int(v.Kind))
			}
		}
	}
}

// TestRouteViolationKindNames pins the short names, which are stable
// identifiers rather than prose.
func TestRouteViolationKindNames(t *testing.T) {
	want := map[RouteViolationKind]string{
		RouteViolationWildcardMidSegment: "wildcard_mid_segment",
		RouteViolationWildcardNotFinal:   "wildcard_not_final",
		RouteViolationExtraWildcard:      "extra_wildcard",
		RouteViolationUnnamedWildcard:    "unnamed_wildcard",
		RouteViolationColonAfterWildcard: "colon_after_wildcard",
		RouteViolationColonAfterParam:    "colon_after_param",
		RouteViolationUnnamedParam:       "unnamed_param",
		RouteViolationColonInStatic:      "colon_in_static",
	}
	for kind, name := range want {
		if got := kind.String(); got != name {
			t.Errorf("RouteViolationKind(%d).String() = %q, want %q", int(kind), got, name)
		}
	}
	if got := RouteViolationKind(0).String(); got != "unknown(0)" {
		t.Errorf("zero kind String() = %q, want %q", got, "unknown(0)")
	}
}
