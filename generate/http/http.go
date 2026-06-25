package http

import (
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	validatepb "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"github.com/go-sphere/protoc-gen-sphere/generate/parser"
	"github.com/go-sphere/protoc-gen-sphere/generate/template"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

const (
	contextPackage  = protogen.GoImportPath("context")
	validatePackage = protogen.GoImportPath("buf.build/go/protovalidate")
)

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

func collectFileHeader(plugin *protogen.Plugin, file *protogen.File) []string {
	return formatFileHeader(
		protocVersion(plugin),
		file.Desc.Path(),
		string(file.GoPackageName),
		file.Proto.GetOptions().GetDeprecated(),
	)
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
	genConf := &GenConfig{
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

func collectGoImport(file *protogen.File, gen *parser.GeneratedFile, conf *Config, genConf *GenConfig) []string {
	lines := make([]string, 0)
	didImport := make(map[protogen.GoImportPath]bool)
	didImport[file.GoImportPath] = true
	for _, ident := range gen.Dummies() {
		if !didImport[ident.GoImportPath] {
			didImport[ident.GoImportPath] = true
			lines = append(lines, fmt.Sprintf("var _  = (*%s)(nil)", gen.QualifiedGoIdent(ident)))
		}
	}
	if !didImport[conf.ServerHandlerFunc.GoImportPath] {
		didImport[conf.ServerHandlerFunc.GoImportPath] = true
		lines = append(lines, fmt.Sprintf("var _  = %s[any]", gen.QualifiedGoIdent(conf.ServerHandlerFunc)))
	}
	if !didImport[conf.DataRespType.GoImportPath] {
		didImport[conf.DataRespType.GoImportPath] = true
		lines = append(lines, fmt.Sprintf("var _  = (*%s[any])(nil)", gen.QualifiedGoIdent(conf.DataRespType)))
	}
LOOP:
	for _, service := range file.Services {
		for _, method := range service.Methods {
			if hasValidateOptionsInMessage(method.Input) || slices.ContainsFunc(method.Input.Fields, hasValidateOptions) {
				ident := validatePackage.Ident("Validate")
				lines = append(lines, fmt.Sprintf("var _ = %s", gen.QualifiedGoIdent(ident)))
				genConf.packageDesc.ValidateFunc = gen.QualifiedGoIdent(ident)
				break LOOP
			}
		}
	}
	return lines
}

func buildServiceDesc(gen *parser.GeneratedFile, service *protogen.Service, conf *GenConfig) (*template.ServiceDesc, error) {
	sd := &template.ServiceDesc{
		ServiceType: service.GoName,
		ServiceName: string(service.Desc.FullName()),
		Package:     conf.packageDesc,
	}
	for _, method := range service.Methods {
		if method.Desc.IsStreamingClient() || method.Desc.IsStreamingServer() {
			logWarn("method `%s.%s` is streaming, it will be ignored. File: `%s`",
				method.Parent.Desc.Name(),
				method.Desc.Name(),
				method.Parent.Location.SourceFile,
			)
			continue
		}
		rule, ok := proto.GetExtension(method.Desc.Options(), annotations.E_Http).(*annotations.HttpRule)
		if rule != nil && ok {
			for _, bind := range rule.AdditionalBindings {
				desc, err := buildHTTPRule(gen, service, method, bind, conf)
				if err != nil {
					return nil, err
				}
				sd.Methods = append(sd.Methods, desc)
			}
			desc, err := buildHTTPRule(gen, service, method, rule, conf)
			if err != nil {
				return nil, err
			}
			sd.Methods = append(sd.Methods, desc)
		} else if !conf.omitempty {
			// Method with no http_rule defined, automatically generating a default POST method.
			path := defaultHTTPPath(conf.omitemptyPrefix, string(service.Desc.FullName()), string(method.Desc.Name()))
			// Body "" with HasBody mirrors ParseHttpRule's normalization of
			// body:"*" (the whole request message is the body).
			res := &parser.HttpRule{
				Path:         path,
				Method:       http.MethodPost,
				HasBody:      true,
				Body:         "",
				ResponseBody: "",
			}
			desc, err := buildMethodDesc(gen, method, res, conf)
			if err != nil {
				return nil, err
			}
			sd.Methods = append(sd.Methods, desc)
		}
	}
	return sd, nil
}

func buildHTTPRule(gen *parser.GeneratedFile, service *protogen.Service, m *protogen.Method, rule *annotations.HttpRule, conf *GenConfig) (*template.MethodDesc, error) {
	res := parser.ParseHttpRule(rule)
	if res.Path == "" {
		res.Path = defaultHTTPPath(conf.omitemptyPrefix, string(service.Desc.FullName()), string(m.Desc.Name()))
	}
	md, err := buildMethodDesc(gen, m, res, conf)
	if err != nil {
		return nil, err
	}
	if _, ok := parser.NoBodyMethods[res.Method]; ok {
		if rule.Body != "" {
			logWarn("method `%s.%s` body should not be declared. File: `%s`",
				m.Parent.Desc.Name(),
				m.Desc.Name(),
				m.Parent.Location.SourceFile,
			)
		}
	} else {
		if rule.Body == "" {
			logWarn("method `%s.%s` body is not declared. File: `%s`",
				m.Parent.Desc.Name(),
				m.Desc.Name(),
				m.Parent.Location.SourceFile,
			)
		}
	}
	return md, nil
}

func buildMethodDesc(gen *parser.GeneratedFile, method *protogen.Method, rule *parser.HttpRule, conf *GenConfig) (*template.MethodDesc, error) {
	route, err := parser.HTTPRoute(rule.Path)
	if err != nil {
		return nil, fmt.Errorf("method `%s.%s` route `%s` parse error: %v. File: `%s`",
			method.Parent.Desc.Name(),
			method.Desc.Name(),
			rule.Path,
			err,
			method.Parent.Location.SourceFile,
		)
	}
	defer func() { conf.methodSets[method.GoName]++ }()

	comment := buildMethodCommend(method)
	needValidate := hasValidateOptionsInMessage(method.Input) || slices.ContainsFunc(method.Input.Fields, hasValidateOptions)

	vars, err := parser.URIParams(method, route)
	if err != nil {
		return nil, err
	}

	forms, err := parser.QueryParams(method, rule.Method, vars)
	if err != nil {
		return nil, err
	}

	headers, err := parser.HeaderParams(method)
	if err != nil {
		return nil, err
	}

	swag := &parser.SwagParams{
		Method:        rule.Method,
		Path:          parser.HTTPRouteToSwaggerRoute(route),
		Auth:          conf.swaggerAuth,
		PathVars:      vars,
		QueryVars:     forms,
		HeaderVars:    headers,
		Body:          rule.Body,
		ResponseBody:  rule.ResponseBody,
		DataResponse:  conf.packageDesc.DataResponseType,
		ErrorResponse: conf.packageDesc.ErrorResponseType,
	}

	swagger, err := parser.BuildAnnotations(gen, method, swag)
	if err != nil {
		return nil, err
	}

	bodyPath := dotPrefixedPath(parser.ProtoKeyPathToGoKeyPath(method.Input, strings.Split(rule.Body, ".")))

	responsePath := dotPrefixedPath(parser.ProtoKeyPathToGoKeyPath(method.Output, strings.Split(rule.ResponseBody, ".")))

	response := gen.QualifiedGoIdent(method.Output.GoIdent)
	if responsePath != "" {
		responseField := parser.ProtoKeyPathToField(method.Output, strings.Split(rule.ResponseBody, "."))
		if responseField == nil {
			return nil, fmt.Errorf("method `%s.%s` field `%s` not found in message `%s`. File: `%s`",
				method.Parent.Desc.Name(),
				method.Desc.Name(),
				responsePath,
				method.Output.Desc.Name(),
				method.Parent.Location.SourceFile,
			)
		}
		response = parser.ProtoTypeToGoType(gen, responseField, true)
	} else {
		response = "*" + response
	}

	return &template.MethodDesc{
		Name:         method.GoName,
		OriginalName: string(method.Desc.Name()),
		Num:          conf.methodSets[method.GoName],
		Comment:      comment,

		Request:  gen.QualifiedGoIdent(method.Input.GoIdent),
		Response: response,
		Reply:    gen.QualifiedGoIdent(method.Output.GoIdent),

		Path:   route,
		Method: rule.Method,

		HasVars:      len(vars) > 0,
		HasQuery:     len(forms) > 0,
		HasBody:      rule.HasBody,
		HasHeader:    len(headers) > 0,
		NeedValidate: needValidate,

		Swagger: swagger,

		Body:         bodyPath,
		ResponseBody: responsePath,
	}, nil
}

func buildMethodCommend(method *protogen.Method) string {
	return formatMethodComment(string(method.Desc.Name()), string(method.Comments.Leading))
}

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

func hasValidateOptions(field *protogen.Field) bool {
	opts := field.Desc.Options().(*descriptorpb.FieldOptions)
	return proto.HasExtension(opts, validatepb.E_Field)
}

func hasValidateOptionsInMessage(msg *protogen.Message) bool {
	return proto.HasExtension(msg.Desc.Options(), validatepb.E_Message)
}
func protocVersion(gen *protogen.Plugin) string {
	return formatProtocVersion(gen.Request.GetCompilerVersion())
}

func logWarn(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "\u001B[31mWARN\u001B[m: "+format+"\n", args...)
}
