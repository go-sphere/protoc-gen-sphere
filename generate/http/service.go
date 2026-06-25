package http

import (
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/parser"
	"github.com/go-sphere/protoc-gen-sphere/generate/internal/template"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

// buildServiceDesc builds the template descriptor for a single service. Methods
// that are streaming, or that lack an http rule while omitempty is enabled, are
// skipped.
func buildServiceDesc(gen *parser.GeneratedFile, service *protogen.Service, conf *genConfig) (*template.ServiceDesc, error) {
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

func buildHTTPRule(gen *parser.GeneratedFile, service *protogen.Service, m *protogen.Method, rule *annotations.HttpRule, conf *genConfig) (*template.MethodDesc, error) {
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

func buildMethodDesc(gen *parser.GeneratedFile, method *protogen.Method, rule *parser.HttpRule, conf *genConfig) (*template.MethodDesc, error) {
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

func logWarn(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "\u001B[31mWARN\u001B[m: "+format+"\n", args...)
}
