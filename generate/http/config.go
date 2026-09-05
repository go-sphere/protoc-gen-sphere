package http

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/template"
	"google.golang.org/protobuf/compiler/protogen"
)

const (
	defaultHTTPxPackage = "github.com/go-sphere/httpx"
	defaultHTTPzPackage = "github.com/go-sphere/sphere/server/httpz"

	// DefaultOmitEmpty controls whether methods without google.api.http rules are
	// omitted.
	DefaultOmitEmpty = true
	// DefaultOmitEmptyPrefix is the route prefix for synthesized default routes.
	DefaultOmitEmptyPrefix = ""
	// DefaultFailOnWarn controls whether generation warnings become errors.
	DefaultFailOnWarn = false

	// DefaultSwaggerAuthHeader is the default swagger auth header comment.
	DefaultSwaggerAuthHeader = `// @Param Authorization header string false "Bearer token"`
	// DefaultContextLoadFunc is the default context load func expression.
	DefaultContextLoadFunc = ".Context()"

	// Default GoIdent flag values, in "import/path;Ident" format. These are the
	// single source of truth shared by main.go's flag defaults and DefaultConfig.
	DefaultRouterType        = defaultHTTPxPackage + ";Router"
	DefaultContextType       = defaultHTTPxPackage + ";Context"
	DefaultHandlerType       = defaultHTTPxPackage + ";Handler"
	DefaultErrorRespType     = defaultHTTPzPackage + ";ErrorResponse"
	DefaultDataRespType      = defaultHTTPzPackage + ";DataResponse"
	DefaultServerHandlerFunc = defaultHTTPzPackage + ";WithJson"
	DefaultStreamHandlerFunc = defaultHTTPzPackage + ";WithSSE"
	DefaultStreamType        = defaultHTTPzPackage + ";SSEStream"
)

// Config controls HTTP server code generation.
type Config struct {
	OmitEmpty       bool
	OmitEmptyPrefix string
	SwaggerAuth     string
	TemplateFile    string
	// FailOnWarn promotes generation warnings (streaming methods that are
	// skipped, GET/DELETE requests declaring a body, missing body declarations)
	// into hard errors so `buf generate` fails instead of silently emitting a
	// partial result. Defaults to false to preserve existing behavior.
	FailOnWarn bool

	RouterType    protogen.GoIdent
	ContextType   protogen.GoIdent
	HandlerType   protogen.GoIdent
	ErrorRespType protogen.GoIdent
	DataRespType  protogen.GoIdent

	ServerHandlerFunc protogen.GoIdent
	// StreamHandlerFunc wraps server-streaming handlers (default
	// httpz.WithSSE). It must accept a two-phase prepare function returning
	// (StreamType[T], error).
	StreamHandlerFunc protogen.GoIdent
	// StreamType is the generic stream type returned by the prepare phase of
	// a streaming handler (default httpz.SSEStream).
	StreamType      protogen.GoIdent
	ContextLoadFunc string
}

// fileConfig holds the per-file generation state derived from Config. It is
// internal to the package and scoped to a single generated file.
type fileConfig struct {
	omitEmpty       bool
	omitEmptyPrefix string
	swaggerAuth     string
	failOnWarn      bool
	packageDesc     *template.PackageDesc
	// serverHandlerFunc is the already-qualified unary wrapper; the stream
	// idents stay unqualified until a server-streaming method needs them, so
	// their imports are only added to files that use them.
	serverHandlerFunc string
	streamHandlerFunc protogen.GoIdent
	streamType        protogen.GoIdent
	// methodSets tracks the per-file duplicate count for each method GoName so
	// MethodDesc.Num stays deterministic. It is scoped to a single generated file
	// (created in generateFileContent) instead of a package global.
	methodSets map[string]int
}

// ParseGoIdent parses an "import/path;Ident" string into a protogen.GoIdent.
func ParseGoIdent(raw string) (protogen.GoIdent, error) {
	importPath, goName, ok := strings.Cut(raw, ";")
	if !ok || importPath == "" || goName == "" || strings.Contains(goName, ";") {
		return protogen.GoIdent{}, errors.New("invalid GoIdent format, expected 'import/path;Ident'")
	}
	return protogen.GoIdent{
		GoName:       goName,
		GoImportPath: protogen.GoImportPath(importPath),
	}, nil
}

// Validate checks that all required generator identifiers are configured.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is required")
	}
	required := []struct {
		name  string
		ident protogen.GoIdent
	}{
		{name: "router_type", ident: c.RouterType},
		{name: "context_type", ident: c.ContextType},
		{name: "handler_type", ident: c.HandlerType},
		{name: "error_resp_type", ident: c.ErrorRespType},
		{name: "data_resp_type", ident: c.DataRespType},
		{name: "server_handler_func", ident: c.ServerHandlerFunc},
		{name: "stream_handler_func", ident: c.StreamHandlerFunc},
		{name: "stream_type", ident: c.StreamType},
	}
	for _, field := range required {
		if field.ident.GoImportPath == "" || field.ident.GoName == "" {
			return fmt.Errorf("%s is required (format: 'import/path;Ident')", field.name)
		}
	}
	return nil
}

// DefaultConfig returns a Config populated with the plugin's default values. It
// is used by main.go (as the basis for flag parsing) and by tests so generated
// output matches real plugin output without re-stating every default.
func DefaultConfig() *Config {
	mustIdent := func(raw string) protogen.GoIdent {
		id, err := ParseGoIdent(raw)
		if err != nil {
			panic(err)
		}
		return id
	}
	return &Config{
		OmitEmpty:         DefaultOmitEmpty,
		OmitEmptyPrefix:   DefaultOmitEmptyPrefix,
		FailOnWarn:        DefaultFailOnWarn,
		SwaggerAuth:       DefaultSwaggerAuthHeader,
		RouterType:        mustIdent(DefaultRouterType),
		ContextType:       mustIdent(DefaultContextType),
		HandlerType:       mustIdent(DefaultHandlerType),
		ErrorRespType:     mustIdent(DefaultErrorRespType),
		DataRespType:      mustIdent(DefaultDataRespType),
		ServerHandlerFunc: mustIdent(DefaultServerHandlerFunc),
		StreamHandlerFunc: mustIdent(DefaultStreamHandlerFunc),
		StreamType:        mustIdent(DefaultStreamType),
		ContextLoadFunc:   DefaultContextLoadFunc,
	}
}
