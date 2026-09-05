// Package http implements the protoc-gen-sphere code generator: it turns a
// proto file's services and google.api.http rules into a .sphere.pb.go file
// containing the HTTP server scaffolding and Swagger annotations.
package http

import (
	"errors"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/parser"
	"github.com/go-sphere/protoc-gen-sphere/generate/internal/template"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

const contextPackage = protogen.GoImportPath("context")

// Generator owns validated configuration and an immutable parsed template.
type Generator struct {
	cfg      *Config
	renderer *template.Renderer
}

// NewGenerator validates cfg and loads its template once for reuse across all
// files in a protoc invocation.
func NewGenerator(cfg *Config) (*Generator, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	renderer, err := template.NewRenderer(cfg.TemplateFile)
	if err != nil {
		return nil, err
	}
	return &Generator{cfg: new(*cfg), renderer: renderer}, nil
}

// GenerateFile is a convenience wrapper for generating one file. Callers that
// generate multiple files should construct a Generator and reuse it.
func GenerateFile(plugin *protogen.Plugin, file *protogen.File, cfg *Config) (*protogen.GeneratedFile, error) {
	generator, err := NewGenerator(cfg)
	if err != nil {
		return nil, err
	}
	return generator.GenerateFile(plugin, file)
}

// GenerateFile generates the .sphere.pb.go file for a single proto file. It
// returns (nil, nil) when the file has no service that needs HTTP code.
func (g *Generator) GenerateFile(plugin *protogen.Plugin, file *protogen.File) (*protogen.GeneratedFile, error) {
	if len(file.Services) == 0 || !hasHTTPRule(g.cfg.OmitEmpty, file.Services) {
		return nil, nil
	}
	filename := file.GeneratedFilenamePrefix + ".sphere.pb.go"
	generated := plugin.NewGeneratedFile(filename, file.GoImportPath)
	if err := generateFileContent(plugin, file, generated, g.cfg, g.renderer); err != nil {
		return nil, err
	}
	return generated, nil
}

func generateFileContent(plugin *protogen.Plugin, file *protogen.File, gen *protogen.GeneratedFile, cfg *Config, renderer *template.Renderer) error {
	if len(file.Services) == 0 {
		return nil
	}

	fileGen := parser.NewGen(gen)
	fileGen.QualifiedGoIdent(contextPackage.Ident("Context"))
	pkgDesc := &template.PackageDesc{
		RouterType:  fileGen.QualifiedGoIdent(cfg.RouterType),
		ContextType: fileGen.QualifiedGoIdent(cfg.ContextType),
		HandlerType: fileGen.QualifiedGoIdent(cfg.HandlerType),

		ErrorResponseType: fileGen.QualifiedGoIdent(cfg.ErrorRespType),
		DataResponseType:  gen.QualifiedGoIdent(cfg.DataRespType),

		ContextLoadFunc: cfg.ContextLoadFunc,
	}
	fileCfg := &fileConfig{
		omitEmpty:       cfg.OmitEmpty,
		omitEmptyPrefix: cfg.OmitEmptyPrefix,
		swaggerAuth:     cfg.SwaggerAuth,
		failOnWarn:      cfg.FailOnWarn,
		packageDesc:     pkgDesc,
		// The unary wrapper is qualified eagerly (collectGoImport keeps its
		// import alive); the stream idents are qualified lazily in
		// buildMethodDesc so files without streaming methods do not import
		// their packages.
		serverHandlerFunc: gen.QualifiedGoIdent(cfg.ServerHandlerFunc),
		streamHandlerFunc: cfg.StreamHandlerFunc,
		streamType:        cfg.StreamType,
		methodSets:        make(map[string]int),
	}

	generateFileHeader(plugin, file, gen)

	var services []*template.ServiceDesc
	for _, service := range file.Services {
		sd, err := buildServiceDesc(fileGen, service, fileCfg)
		if err != nil {
			return err
		}
		if len(sd.Methods) == 0 {
			continue
		}
		services = append(services, sd)
	}
	importLines := collectGoImport(file, fileGen, cfg, fileCfg)
	for _, line := range importLines {
		gen.P(line)
	}
	gen.P()
	for _, sd := range services {
		content, err := renderer.Execute(sd)
		if err != nil {
			return err
		}
		gen.P(content)
		gen.P("\n\n")
	}
	return nil
}

// hasHTTPRule reports whether any method in services needs HTTP code.
// Client-streaming and bidirectional methods never qualify (they have no
// HTTP/1.1 mapping); unary and server-streaming methods do. When omitEmpty is
// false every such method qualifies (a default route is synthesized);
// otherwise only methods carrying a google.api.http rule do.
func hasHTTPRule(omitEmpty bool, services []*protogen.Service) bool {
	for _, service := range services {
		for _, method := range service.Methods {
			if method.Desc.IsStreamingClient() {
				continue
			}
			if !omitEmpty {
				return true
			}
			ext := proto.GetExtension(method.Desc.Options(), annotations.E_Http)
			if ext == nil {
				continue
			}
			rule, ok := ext.(*annotations.HttpRule)
			if rule != nil && ok {
				return true
			}
		}
	}
	return false
}

func protocVersion(gen *protogen.Plugin) string {
	return formatProtocVersion(gen.Request.GetCompilerVersion())
}
