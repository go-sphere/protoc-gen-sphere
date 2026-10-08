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
		hasRule := rule != nil && ok
		if hasRule || !cfg.omitEmpty {
			if err := checkRequestOneof(method); err != nil {
				return nil, err
			}
		}
		if hasRule {
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
	sd.MethodSets = template.IndexMethods(sd.Methods)
	sd.DistinctMethods = template.DistinctMethods(sd.Methods)
	return sd, nil
}

// checkRequestOneof rejects a request message that contains a real oneof,
// directly or in a nested message. Generated handlers cannot bind one: the JSON
// body is decoded with encoding/json, which ignores the interface-typed field
// protoc-gen-go emits for a oneof, and the query/uri/header/form binders skip
// oneof members. Proto3 optional fields (synthetic oneofs) are fine.
func checkRequestOneof(method *protogen.Method) error {
	found, ok := parser.FindRequestOneof(method.Input)
	if !ok {
		return nil
	}
	where := "request message"
	if found.Path != "" {
		where = fmt.Sprintf("request field `%s`", found.Path)
	}
	return fmt.Errorf("method `%s.%s` %s contains oneof `%s` (message `%s`), which generated HTTP handlers cannot bind: the JSON body is decoded with encoding/json, which ignores oneof fields, and query/uri/header/form binding skips them; replace the oneof with separate optional fields. File: `%s`, Message: `%s`",
		method.Parent.Desc.Name(),
		method.Desc.Name(),
		where,
		found.Oneof.Desc.Name(),
		found.Oneof.Parent.Desc.FullName(),
		method.Parent.Location.SourceFile,
		method.Input.Desc.Name(),
	)
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
	// httpx promises three path shapes and nothing else; anything outside them
	// is unspecified, and the five adapters disagree about it. Warn rather than
	// reject: the upstream policy is "no restriction, and no promise", so the
	// route is still emitted exactly as converted.
	for _, v := range parser.RouteViolations(route) {
		if err := cfg.warn("method `%s.%s` route `%s` is outside the route grammar httpx promises: %s. File: `%s`",
			method.Parent.Desc.Name(),
			method.Desc.Name(),
			route,
			v.Detail,
			method.Parent.Location.SourceFile,
		); err != nil {
			return nil, err
		}
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

	if err := checkTopLevelField(method, method.Input, "body", rule.Body); err != nil {
		return nil, err
	}
	if err := checkTopLevelField(method, method.Output, "response_body", rule.ResponseBody); err != nil {
		return nil, err
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

	if rule.HasBody && rule.Body != "" && len(forms) == 0 {
		if err := warnFieldsOutsideBody(method, rule.Body, cfg); err != nil {
			return nil, err
		}
	}
	// With body:"*" and no JSON-located field there is nothing to decode.
	// Skipping BindJSON keeps an empty body valid on every adapter, and the
	// Swagger docs then declare no request body.
	if rule.HasBody && rule.Body == "" && len(parser.JSONFields(method.Input)) == 0 {
		rule.HasBody = false
	}

	swag := &parser.SwagParams{
		Method:        rule.Method,
		Path:          parser.HTTPRouteToSwaggerRoute(route),
		Auth:          cfg.swaggerAuth,
		HasBody:       rule.HasBody,
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
	responseZero := goZeroValue(response)

	hasBody := rule.HasBody && len(forms) == 0
	if hasBody && cfg.packageDesc.ErrorsIsFunc == "" {
		cfg.packageDesc.ErrorsIsFunc = g.QualifiedUsedGoIdent(protogen.GoImportPath("errors").Ident("Is"))
		cfg.packageDesc.EOFVar = g.QualifiedUsedGoIdent(protogen.GoImportPath("io").Ident("EOF"))
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
		// The zero value keeps error returns compilable when response_body
		// projects the reply onto a scalar (value-type) field.
		ResponseZero: responseZero,
		Reply:        g.QualifiedGoIdent(method.Output.GoIdent),

		Path:   route,
		Method: rule.Method,

		HasVars:      len(vars) > 0,
		HasQuery:     len(queries) > 0,
		HasForm:      len(forms) > 0,
		HasBody:      hasBody,
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

// checkTopLevelField rejects a body or response_body selector that is not the
// name of a top-level field of message. google.api.http requires a top-level
// field; a nested path would make the generated handler dereference a nil
// intermediate message, and an unknown name would bind the whole message.
func checkTopLevelField(method *protogen.Method, message *protogen.Message, option, path string) error {
	if path == "" {
		return nil
	}
	if strings.Contains(path, ".") {
		return fmt.Errorf("method `%s.%s` %s `%s` is a nested field path; google.api.http requires a top-level field of message `%s`. File: `%s`",
			method.Parent.Desc.Name(),
			method.Desc.Name(),
			option,
			path,
			message.Desc.Name(),
			method.Parent.Location.SourceFile,
		)
	}
	if parser.ProtoKeyPathToField(message, []string{path}) == nil {
		return fmt.Errorf("method `%s.%s` %s `%s` is not a field of message `%s`. File: `%s`",
			method.Parent.Desc.Name(),
			method.Desc.Name(),
			option,
			path,
			message.Desc.Name(),
			method.Parent.Location.SourceFile,
		)
	}
	return nil
}

// warnFieldsOutsideBody reports the request fields left without a source when
// body names a single field: the JSON body decodes only into that field, so the
// other JSON-located fields (explicit or by default) always stay zero.
func warnFieldsOutsideBody(method *protogen.Method, body string, cfg *fileConfig) error {
	for _, field := range parser.JSONFields(method.Input) {
		if string(field.Desc.Name()) == body {
			continue
		}
		if err := cfg.warn("method `%s.%s` field `%s` is bound to the JSON body, but body is `%s`, so the field is never bound; give it a QUERY, URI or HEADER location, or use body: \"*\". File: `%s`, Message: `%s`",
			method.Parent.Desc.Name(),
			method.Desc.Name(),
			field.Desc.Name(),
			body,
			method.Parent.Location.SourceFile,
			method.Input.Desc.Name(),
		); err != nil {
			return err
		}
	}
	return nil
}

func buildMethodComment(method *protogen.Method) string {
	return formatMethodComment(string(method.Desc.Name()), string(method.Comments.Leading))
}

// goZeroValue returns the zero-value expression for a Go type expression as
// rendered by parser.ProtoTypeToGoType: pointers, slices and maps zero to
// nil, strings to "", bools to false, and every numeric or named enum type
// accepts the untyped constant 0.
func goZeroValue(goType string) string {
	switch {
	case strings.HasPrefix(goType, "*"),
		strings.HasPrefix(goType, "[]"),
		strings.HasPrefix(goType, "map["),
		goType == "any":
		return "nil"
	case goType == "string":
		return `""`
	case goType == "bool":
		return "false"
	default:
		return "0"
	}
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
