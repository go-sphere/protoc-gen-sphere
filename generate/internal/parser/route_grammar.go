package parser

import (
	"fmt"
	"strings"
)

// The route grammar httpx promises is three shapes and nothing else: a static
// segment, a ":name" parameter filling exactly one segment, and a single
// "*name" wildcard as the final segment. Anything else gets no restriction and
// no promise — it may panic at registration, match requests it should not, or
// silently collide with a sibling route, and the five adapters disagree about
// which. stdx is the reference implementation: what its router accepts is what
// httpx promises.
//
// The contract is defined upstream, in github.com/go-sphere/httpx: the doc
// comment on httpx.Registrar (router.go) states the grammar, and
// httpx.ValidateWildcardPath (wildcard.go) enforces the wildcard half of it
// from every adapter's registration entry point, so every adapter panics
// identically on a bad wildcard.
//
// The rules are mirrored here rather than imported. This plugin does not
// depend on httpx — it only names it in default configuration strings — and
// coupling a codegen plugin's version to the runtime library's would be a real
// cost. Keep the two in sync by hand. The per-adapter behavior quoted in the
// violation details was measured against the httpx v0.0.5 adapters.

// RouteViolationKind classifies the ways a route can fall outside the grammar.
type RouteViolationKind int

const (
	// RouteViolationWildcardMidSegment is a '*' that does not start its own
	// path segment ("/v1/fo*o").
	RouteViolationWildcardMidSegment RouteViolationKind = iota + 1
	// RouteViolationWildcardNotFinal is a wildcard segment followed by more
	// segments ("/v1/*path/more").
	RouteViolationWildcardNotFinal
	// RouteViolationExtraWildcard is a second wildcard ("/v1/*a/x/*b").
	RouteViolationExtraWildcard
	// RouteViolationUnnamedWildcard is a wildcard with no name ("/v1/files/*").
	RouteViolationUnnamedWildcard
	// RouteViolationColonAfterWildcard is a literal ':' inside a wildcard
	// segment ("/v1/files/*path:archive").
	RouteViolationColonAfterWildcard
	// RouteViolationColonAfterParam is a literal ':' inside a parameter
	// segment ("/v1/users/:id:activate").
	RouteViolationColonAfterParam
	// RouteViolationUnnamedParam is a parameter with no name ("/v1/:").
	RouteViolationUnnamedParam
	// RouteViolationColonInStatic is a literal ':' inside a static segment —
	// the google.api.http custom-method style ("/v1/reports:generate").
	RouteViolationColonInStatic
)

// String returns a stable short name for the kind.
func (k RouteViolationKind) String() string {
	switch k {
	case RouteViolationWildcardMidSegment:
		return "wildcard_mid_segment"
	case RouteViolationWildcardNotFinal:
		return "wildcard_not_final"
	case RouteViolationExtraWildcard:
		return "extra_wildcard"
	case RouteViolationUnnamedWildcard:
		return "unnamed_wildcard"
	case RouteViolationColonAfterWildcard:
		return "colon_after_wildcard"
	case RouteViolationColonAfterParam:
		return "colon_after_param"
	case RouteViolationUnnamedParam:
		return "unnamed_param"
	case RouteViolationColonInStatic:
		return "colon_in_static"
	default:
		return fmt.Sprintf("unknown(%d)", int(k))
	}
}

// RouteViolation is one way a route leaves the promised grammar.
type RouteViolation struct {
	// Kind classifies the violation.
	Kind RouteViolationKind
	// Segment is the offending path segment, without its leading slash.
	Segment string
	// Detail says what is wrong with the segment and what the adapters do
	// with it at runtime. It is a sentence fragment, meant to be embedded in
	// a warning that already names the method, the route and the source file.
	Detail string
}

// panicsEverywhere is the consequence shared by every wildcard shape
// ValidateWildcardPath rejects: all five adapters call it from every
// registration entry point, so all five panic with that same error.
const panicsEverywhere = "every adapter panics at registration (httpx.ValidateWildcardPath)"

// RouteViolations reports every way route — a path already converted by
// HTTPRoute — falls outside the grammar httpx promises. It returns nil for a
// route inside the grammar. At most one violation of each kind is reported,
// in path order, so a route that repeats a mistake warns once about it.
//
// This is a diagnostic only. httpx deliberately does not reject these paths,
// and neither does this generator: the route is emitted either way.
func RouteViolations(route string) []RouteViolation {
	segments := strings.Split(strings.TrimPrefix(route, "/"), "/")
	last := len(segments) - 1

	var (
		found        []RouteViolation
		seen         = make(map[RouteViolationKind]bool, len(segments))
		wildcardSeen bool
	)
	report := func(kind RouteViolationKind, segment, format string, args ...any) {
		if seen[kind] {
			return
		}
		seen[kind] = true
		found = append(found, RouteViolation{
			Kind:    kind,
			Segment: segment,
			Detail:  fmt.Sprintf(format, args...),
		})
	}

	for i, seg := range segments {
		if seg == "" {
			// HTTPRoute collapses repeated slashes, so this is only the
			// root path. An empty static segment is not a grammar violation.
			continue
		}
		switch {
		case strings.HasPrefix(seg, "*"):
			name := seg[1:]
			switch {
			case wildcardSeen:
				report(RouteViolationExtraWildcard, seg,
					"wildcard `%s` is a second wildcard and a route may have only one; %s",
					seg, panicsEverywhere)
			case i != last:
				report(RouteViolationWildcardNotFinal, seg,
					"wildcard `%s` is not the final path segment; %s",
					seg, panicsEverywhere)
			}
			wildcardSeen = true
			switch {
			case name == "":
				report(RouteViolationUnnamedWildcard, seg,
					"wildcard `*` has no name; %s — write `*name`", panicsEverywhere)
			case strings.Contains(name, "*"):
				report(RouteViolationExtraWildcard, seg,
					"segment `%s` holds two wildcards and a route may have only one; %s",
					seg, panicsEverywhere)
			case strings.Contains(name, ":"):
				report(RouteViolationColonAfterWildcard, seg,
					"wildcard segment `%s` contains a literal ':'; ginx panics at registration, "+
						"and echox, fiberx, hertzx and stdx name the wildcard `%s`, so `%s` is unreachable",
					seg, name, name[:strings.IndexByte(name, ':')])
			}
		case strings.HasPrefix(seg, ":"):
			name := seg[1:]
			switch {
			case strings.Contains(seg, "*"):
				report(RouteViolationWildcardMidSegment, seg,
					"segment `%s` has a '*' that does not start its own path segment; %s",
					seg, panicsEverywhere)
			case name == "" || strings.HasPrefix(name, ":"):
				report(RouteViolationUnnamedParam, seg,
					"parameter `%s` has no name; ginx and hertzx panic at registration, "+
						"and echox, fiberx and stdx key the captured segment under the empty string",
					seg)
			case strings.Contains(name, ":"):
				report(RouteViolationColonAfterParam, seg,
					"parameter segment `%s` contains a second ':'; ginx and hertzx panic at registration, "+
						"echox and stdx capture one parameter named `%s`, and fiberx splits the segment in two, "+
						"so no adapter serves the route as written",
					seg, name)
			}
		default:
			if strings.Contains(seg, "*") {
				report(RouteViolationWildcardMidSegment, seg,
					"segment `%s` has a '*' that does not start its own path segment; %s",
					seg, panicsEverywhere)
			}
			if idx := strings.IndexByte(seg, ':'); idx >= 0 {
				report(RouteViolationColonInStatic, seg,
					"static segment `%s` contains a literal ':' (google.api.http custom-method style); "+
						"stdx matches it literally, while ginx, echox, fiberx and hertzx read `%s` as a path "+
						"parameter so sibling paths match too, and two such routes under one prefix panic at "+
						"registration on ginx and hertzx while echox and fiberx register both and send every "+
						"path under the prefix to one of them",
					seg, seg[idx:])
			}
		}
	}
	return found
}
