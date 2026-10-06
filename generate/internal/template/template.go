// Package template renders the HTTP server scaffolding emitted by
// protoc-gen-sphere.
package template

import (
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/template"
)

//go:embed template.tmpl
var defaultTemplate string

/*
service TestService {
  rpc RunTest(RunTestRequest) returns (RunTestResponse) {
    option (google.api.http) = {
      post: "/api/test/{path_test1}/second/{path_test2}"
      body: "*"
    };
  }
}
*/

// ServiceDesc is the template model for one generated HTTP service.
type ServiceDesc struct {
	ServiceType string // TestService
	ServiceName string // shared.v1.TestService

	// Methods holds one entry per HTTP binding (additional bindings of the
	// same rpc share a Name and differ by Num), in declaration order.
	Methods []*MethodDesc
	// MethodSets indexes Methods by Name (see IndexMethods). Ranging over it
	// visits names in sorted order; kept for custom templates.
	MethodSets map[string]*MethodDesc
	// DistinctMethods holds one entry per Name in declaration order (see
	// DistinctMethods), for per-rpc sections such as the server interface.
	DistinctMethods []*MethodDesc

	Package *PackageDesc
}

// MethodDesc is the template model for one generated HTTP method.
type MethodDesc struct {
	// method
	Name         string // rpc method name: RunTest
	OriginalName string // service and method name: TestServiceRunTest
	Num          int    // duplicate method number, used for generating unique method names
	Comment      string // leading comment for the method

	Request  string // rpc request type
	Reply    string // rpc reply type
	Response string // http response type
	// ResponseZero is the zero-value expression for Response, used in error
	// returns: "nil" for pointer/slice/map types, "" for string, "false" for
	// bool and "0" for numeric and enum types.
	ResponseZero string

	// http_rule
	Path   string // gin route: /api/test/:path_test1/second/:path_test
	Method string // POST

	HasVars      bool
	HasQuery     bool
	HasForm      bool
	HasBody      bool
	HasHeader    bool
	NeedValidate bool

	// IsServerStream marks a server-streaming method: the generated
	// interface takes a push callback and the handler wraps a two-phase
	// prepare/stream function instead of a (T, error) function.
	IsServerStream bool
	// HandlerWrapperFunc is the qualified wrapper applied to this method's
	// handler (e.g. httpz.WithJson for unary, httpz.WithSSE for streams).
	HandlerWrapperFunc string
	// StreamType is the qualified generic stream type returned by the
	// prepare phase (e.g. httpz.SSEStream). Set only for server streams.
	StreamType string

	Swagger string

	Body         string
	ResponseBody string
}

// PackageDesc contains the qualified package-level identifiers used by a
// generated HTTP service.
type PackageDesc struct {
	RouterType  string
	ContextType string
	HandlerType string

	ErrorResponseType string
	DataResponseType  string

	ValidateFunc string

	ContextLoadFunc string
}

// Renderer owns a parsed HTTP generation template. It is immutable after
// construction and safe to reuse for every file in one plugin invocation.
type Renderer struct {
	template *template.Template
}

// NewRenderer loads and parses the embedded template, or the file at path when
// path is non-empty.
func NewRenderer(path string) (*Renderer, error) {
	source := defaultTemplate
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read template %q: %w", path, err)
		}
		source = string(raw)
	}
	// goString quotes dynamic values (paths, custom HTTP methods, operation
	// names) into valid Go string literals (guidelines §7.3).
	tmpl, err := template.New("http").Funcs(template.FuncMap{
		"goString": strconv.Quote,
	}).Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return &Renderer{template: tmpl}, nil
}

// IndexMethods returns the MethodSets index for methods, keyed by method Name.
// When several descriptors share a Name (e.g. additional bindings), the last
// one wins. Descriptor builders call it so Execute stays pure rendering.
func IndexMethods(methods []*MethodDesc) map[string]*MethodDesc {
	sets := make(map[string]*MethodDesc, len(methods))
	for _, m := range methods {
		sets[m.Name] = m
	}
	return sets
}

// DistinctMethods returns methods de-duplicated by Name, ordered by each
// Name's first occurrence (declaration order). Each entry is the same
// descriptor IndexMethods selects for that Name (the last one), so ranging
// over the result renders exactly what ranging over MethodSets does, only in
// declaration order instead of sorted by Name.
func DistinctMethods(methods []*MethodDesc) []*MethodDesc {
	pos := make(map[string]int, len(methods))
	out := make([]*MethodDesc, 0, len(methods))
	for _, m := range methods {
		if i, ok := pos[m.Name]; ok {
			out[i] = m
			continue
		}
		pos[m.Name] = len(out)
		out = append(out, m)
	}
	return out
}

// Execute renders a service descriptor. It does not modify s; callers must
// populate MethodSets and DistinctMethods (see IndexMethods and
// DistinctMethods) before rendering.
func (r *Renderer) Execute(s *ServiceDesc) (string, error) {
	var buf strings.Builder
	if err := r.template.Execute(&buf, s); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}
