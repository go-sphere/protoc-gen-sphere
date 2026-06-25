package http

import (
	"errors"
	"strings"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/template"
	"google.golang.org/protobuf/compiler/protogen"
)

const (
	defaultHTTPxPackage = "github.com/go-sphere/httpx"
	defaultHTTPzPackage = "github.com/go-sphere/sphere/server/httpz"

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
)

type Config struct {
	Omitempty       bool
	OmitemptyPrefix string
	SwaggerAuth     string
	TemplateFile    string

	RouterType    protogen.GoIdent
	ContextType   protogen.GoIdent
	HandlerType   protogen.GoIdent
	ErrorRespType protogen.GoIdent
	DataRespType  protogen.GoIdent

	ServerHandlerFunc protogen.GoIdent
	ContextLoadFunc   string
}

// genConfig holds the per-file generation state derived from Config. It is
// internal to the package and scoped to a single generated file.
type genConfig struct {
	omitempty       bool
	omitemptyPrefix string
	swaggerAuth     string
	packageDesc     *template.PackageDesc
	// methodSets tracks the per-file duplicate count for each method GoName so
	// MethodDesc.Num stays deterministic. It is scoped to a single generated file
	// (created in generateFileContent) instead of a package global.
	methodSets map[string]int
}

// ParseGoIdent parses a "import/path;Ident" string into a protogen.GoIdent.
func ParseGoIdent(raw string) (protogen.GoIdent, error) {
	parts := strings.Split(raw, ";")
	if len(parts) != 2 {
		return protogen.GoIdent{}, errors.New("invalid GoIdent format, expected 'path;ident'")
	}
	return protogen.GoIdent{
		GoName:       parts[1],
		GoImportPath: protogen.GoImportPath(parts[0]),
	}, nil
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
		Omitempty:         true,
		SwaggerAuth:       DefaultSwaggerAuthHeader,
		RouterType:        mustIdent(DefaultRouterType),
		ContextType:       mustIdent(DefaultContextType),
		HandlerType:       mustIdent(DefaultHandlerType),
		ErrorRespType:     mustIdent(DefaultErrorRespType),
		DataRespType:      mustIdent(DefaultDataRespType),
		ServerHandlerFunc: mustIdent(DefaultServerHandlerFunc),
		ContextLoadFunc:   DefaultContextLoadFunc,
	}
}
