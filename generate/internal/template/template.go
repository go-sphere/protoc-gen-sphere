// Package template renders the HTTP server scaffolding emitted by
// protoc-gen-sphere.
package template

import (
	_ "embed"
	"fmt"
	"os"
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

	Methods    []*MethodDesc
	MethodSets map[string]*MethodDesc

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

	// http_rule
	Path   string // gin route: /api/test/:path_test1/second/:path_test
	Method string // POST

	HasVars      bool
	HasQuery     bool
	HasForm      bool
	HasBody      bool
	HasHeader    bool
	NeedValidate bool

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

	ServerHandlerWrapperFunc string
	ContextLoadFunc          string
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
	tmpl, err := template.New("http").Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return &Renderer{template: tmpl}, nil
}

// Execute renders a service descriptor.
func (r *Renderer) Execute(s *ServiceDesc) (string, error) {
	s.MethodSets = make(map[string]*MethodDesc)
	for _, m := range s.Methods {
		s.MethodSets[m.Name] = m
	}
	var buf strings.Builder
	if err := r.template.Execute(&buf, s); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}
