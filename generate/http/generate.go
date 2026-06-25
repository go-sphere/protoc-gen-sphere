// Package http implements the protoc-gen-sphere code generator: it turns a
// proto file's services and google.api.http rules into a .sphere.pb.go file
// containing the HTTP server scaffolding and Swagger annotations.
package http

import (
	"github.com/go-sphere/protoc-gen-sphere/generate/internal/parser"
	"github.com/go-sphere/protoc-gen-sphere/generate/internal/template"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

const contextPackage = protogen.GoImportPath("context")

// ReplaceTemplateIfNeed overrides the built-in code template with the file at
// path when path is non-empty. It must be called once before GenerateFile. It
// is a thin wrapper over the internal template package so that callers (e.g.
// main) need not import that internal package directly.
func ReplaceTemplateIfNeed(path string) error {
	return template.ReplaceTemplateIfNeed(path)
}

// GenerateFile generates the .sphere.pb.go file for a single proto file. It
// returns (nil, nil) when the file has no service that needs HTTP code.
func GenerateFile(plugin *protogen.Plugin, file *protogen.File, conf *Config) (*protogen.GeneratedFile, error) {
	if len(file.Services) == 0 || !hasHTTPRule(conf.Omitempty, file.Services) {
		return nil, nil
	}
	filename := file.GeneratedFilenamePrefix + ".sphere.pb.go"
	g := plugin.NewGeneratedFile(filename, file.GoImportPath)
	err := generateFileContent(plugin, file, g, conf)
	if err != nil {
		return nil, err
	}
	return g, nil
}

func generateFileContent(plugin *protogen.Plugin, file *protogen.File, gen *protogen.GeneratedFile, conf *Config) error {
	if len(file.Services) == 0 {
		return nil
	}

	fileGen := parser.NewGen(gen)
	fileGen.QualifiedGoIdent(contextPackage.Ident("Context"))
	pkgDesc := &template.PackageDesc{
		RouterType:  fileGen.QualifiedGoIdent(conf.RouterType),
		ContextType: fileGen.QualifiedGoIdent(conf.ContextType),
		HandlerType: fileGen.QualifiedGoIdent(conf.HandlerType),

		ErrorResponseType: fileGen.QualifiedGoIdent(conf.ErrorRespType),
		DataResponseType:  gen.QualifiedGoIdent(conf.DataRespType),

		ServerHandlerWrapperFunc: gen.QualifiedGoIdent(conf.ServerHandlerFunc),
		ContextLoadFunc:          conf.ContextLoadFunc,
	}
	genConf := &genConfig{
		omitempty:       conf.Omitempty,
		omitemptyPrefix: conf.OmitemptyPrefix,
		swaggerAuth:     conf.SwaggerAuth,
		packageDesc:     pkgDesc,
		methodSets:      make(map[string]int),
	}

	headerLines := collectFileHeader(plugin, file)
	for _, line := range headerLines {
		gen.P(line)
	}

	var services []*template.ServiceDesc
	for _, service := range file.Services {
		sd, err := buildServiceDesc(fileGen, service, genConf)
		if err != nil {
			return err
		}
		if len(sd.Methods) == 0 {
			continue
		}
		services = append(services, sd)
	}
	importLines := collectGoImport(file, fileGen, conf, genConf)
	for _, line := range importLines {
		gen.P(line)
	}
	gen.P()
	for _, sd := range services {
		content, err := sd.Execute()
		if err != nil {
			return err
		}
		gen.P(content)
		gen.P("\n\n")
	}
	return nil
}

func collectFileHeader(plugin *protogen.Plugin, file *protogen.File) []string {
	return formatFileHeader(
		protocVersion(plugin),
		file.Desc.Path(),
		string(file.GoPackageName),
		file.Proto.GetOptions().GetDeprecated(),
	)
}

// hasHTTPRule reports whether any non-streaming method in services needs HTTP
// code. When omitempty is false every method qualifies (a default route is
// synthesized); otherwise only methods carrying a google.api.http rule do.
func hasHTTPRule(omitempty bool, services []*protogen.Service) bool {
	for _, service := range services {
		for _, method := range service.Methods {
			if method.Desc.IsStreamingClient() || method.Desc.IsStreamingServer() {
				continue
			}
			if !omitempty {
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
