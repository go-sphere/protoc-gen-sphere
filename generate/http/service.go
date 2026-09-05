package http

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/parser"
	"github.com/go-sphere/protoc-gen-sphere/generate/internal/template"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

// buildServiceDesc builds the template descriptor for a single service.
// Server-streaming methods generate SSE handlers; client-streaming and
// bidirectional methods, and methods that lack an HTTP rule while omit-empty
// is enabled, are skipped.
func buildServiceDesc(g *parser.GeneratedFile, service *protogen.Service, cfg *fileConfig) (*template.ServiceDesc, error) {
	sd := &template.ServiceDesc{
		ServiceType: service.GoName,
		ServiceName: string(service.Desc.FullName()),
		Package:     cfg.packageDesc,
	}
	for _, method := range service.Methods {
		if method.Desc.IsStreamingClient() {
			// Client and bidirectional streams have no HTTP/1.1 mapping;
			// server-only streams map to Server-Sent Events and proceed.
			if err := cfg.warn("method `%s.%s` is client/bidirectional streaming, it will be ignored. File: `%s`",
				method.Parent.Desc.Name(),
				method.Desc.Name(),
				method.Parent.Location.SourceFile,
			); err != nil {
				return nil, err
			}
			continue
		}
		rule, ok := proto.GetExtension(method.Desc.Options(), annotations.E_Http).(*annotations.HttpRule)
		if rule != nil && ok {
			for _, bind := range rule.AdditionalBindings {
				desc, err := buildHTTPRule(g, service, method, bind, cfg)
				if err != nil {
					return nil, err
				}
				sd.Methods = append(sd.Methods, desc)
			}
			desc, err := buildHTTPRule(g, service, method, rule, cfg)
			if err != nil {
				return nil, err
			}
			sd.Methods = append(sd.Methods, desc)
		} else if !cfg.omitEmpty {
			// Method with no http_rule defined, automatically generating a default POST method.
			path := defaultHTTPPath(cfg.omitEmptyPrefix, string(service.Desc.FullName()), string(method.Desc.Name()))
			// Body "" with HasBody mirrors ParseHttpRule's normalization of
			// body:"*" (the whole request message is the body).
			res := &parser.HttpRule{
				Path:         path,
				Method:       http.MethodPost,
				HasBody:      true,
				Body:         "",
				ResponseBody: "",
			}
			desc, err := buildMethodDesc(g, method, res, cfg)
			if err != nil {
				return nil, err
			}
			sd.Methods = append(sd.Methods, desc)
		}
	}
	return sd, nil
}

func buildHTTPRule(g *parser.GeneratedFile, service *protogen.Service, method *protogen.Method, rule *annotations.HttpRule, cfg *fileConfig) (*template.MethodDesc, error) {
	res := parser.ParseHttpRule(rule)
	if res.Path == "" {
		res.Path = defaultHTTPPath(cfg.omitEmptyPrefix, string(service.Desc.FullName()), string(method.Desc.Name()))
	}
	forms, err := parser.FormParams(method)
	if err != nil {
		return nil, err
	}
	if _, ok := parser.NoBodyMethods[res.Method]; ok {
		if rule.Body != "" {
			if err := cfg.warn("method `%s.%s` body should not be declared. File: `%s`",
				method.Parent.Desc.Name(),
				method.Desc.Name(),
				method.Parent.Location.SourceFile,
			); err != nil {
				return nil, err
			}
		}
		// GET/HEAD/DELETE/OPTIONS have no request body: even if a body was
		// declared, force it off so the generated handler does not emit an
		// unusable ctx.BindJSON call.
		res.HasBody = false
		res.Body = ""
	} else if len(forms) > 0 {
		if rule.Body != "" {
			if err := cfg.warn("method `%s.%s` body should not be declared when form parameters are present. File: `%s`",
				method.Parent.Desc.Name(),
				method.Desc.Name(),
				method.Parent.Location.SourceFile,
			); err != nil {
				return nil, err
			}
		}
		res.HasBody = false
		res.Body = ""
	} else if rule.Body == "" {
		if err := cfg.warn("method `%s.%s` body is not declared. File: `%s`",
			method.Parent.Desc.Name(),
			method.Desc.Name(),
			method.Parent.Location.SourceFile,
		); err != nil {
			return nil, err
		}
	}
	md, err := buildMethodDesc(g, method, res, cfg)
	if err != nil {
		return nil, err
	}
	return md, nil
}

func buildMethodDesc(g *parser.GeneratedFile, method *protogen.Method, rule *parser.HttpRule, cfg *fileConfig) (*template.MethodDesc, error) {
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
	defer func() { cfg.methodSets[method.GoName]++ }()

	comment := buildMethodComment(method)
	needValidate := requestNeedsValidate(method.Input)

	isServerStream := method.Desc.IsStreamingServer()
	if isServerStream && rule.ResponseBody != "" {
		// A stream delivers whole reply messages as events; there is no
		// single response to project a field out of.
		if err := cfg.warn("method `%s.%s` response_body is ignored on server-streaming methods. File: `%s`",
			method.Parent.Desc.Name(),
			method.Desc.Name(),
			method.Parent.Location.SourceFile,
		); err != nil {
			return nil, err
		}
		rule.ResponseBody = ""
	}

	vars, err := parser.URIParams(method, route)
	if err != nil {
		return nil, err
	}

	queries, err := parser.QueryParams(method, rule.Method, vars)
	if err != nil {
		return nil, err
	}

	forms, err := parser.FormParams(method)
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
		Auth:          cfg.swaggerAuth,
		PathVars:      vars,
		QueryVars:     queries,
		FormVars:      forms,
		HeaderVars:    headers,
		Body:          rule.Body,
		ResponseBody:  rule.ResponseBody,
		DataResponse:  cfg.packageDesc.DataResponseType,
		ErrorResponse: cfg.packageDesc.ErrorResponseType,
		Stream:        isServerStream,
	}

	swagger, err := parser.BuildAnnotations(g, method, swag)
	if err != nil {
		return nil, err
	}

	bodyPath := dotPrefixedPath(parser.ProtoKeyPathToGoKeyPath(method.Input, strings.Split(rule.Body, ".")))

	responsePath := dotPrefixedPath(parser.ProtoKeyPathToGoKeyPath(method.Output, strings.Split(rule.ResponseBody, ".")))

	response := g.QualifiedGoIdent(method.Output.GoIdent)
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
		response = parser.ProtoTypeToGoType(g, responseField, true)
	} else {
		response = "*" + response
	}

	handlerWrapper := cfg.serverHandlerFunc
	streamType := ""
	if isServerStream {
		handlerWrapper = g.QualifiedGoIdent(cfg.streamHandlerFunc)
		streamType = g.QualifiedGoIdent(cfg.streamType)
	}

	return &template.MethodDesc{
		Name:         method.GoName,
		OriginalName: string(method.Desc.Name()),
		Num:          cfg.methodSets[method.GoName],
		Comment:      comment,

		Request:  g.QualifiedGoIdent(method.Input.GoIdent),
		Response: response,
		Reply:    g.QualifiedGoIdent(method.Output.GoIdent),

		Path:   route,
		Method: rule.Method,

		HasVars:      len(vars) > 0,
		HasQuery:     len(queries) > 0,
		HasForm:      len(forms) > 0,
		HasBody:      rule.HasBody && len(forms) == 0,
		HasHeader:    len(headers) > 0,
		NeedValidate: needValidate,

		IsServerStream:     isServerStream,
		HandlerWrapperFunc: handlerWrapper,
		StreamType:         streamType,

		Swagger: swagger,

		Body:         bodyPath,
		ResponseBody: responsePath,
	}, nil
}

func buildMethodComment(method *protogen.Method) string {
	return formatMethodComment(string(method.Desc.Name()), string(method.Comments.Leading))
}

// warn reports a generation warning. When failOnWarn is enabled the warning is
// promoted to a hard error so the caller (buf generate) fails; otherwise it is
// printed to stderr and generation continues with the pre-existing behavior.
func (c *fileConfig) warn(format string, args ...any) error {
	if c.failOnWarn {
		return fmt.Errorf(format, args...)
	}
	logWarn(format, args...)
	return nil
}

func logWarn(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "\u001B[31mWARN\u001B[m: "+format+"\n", args...)
}
