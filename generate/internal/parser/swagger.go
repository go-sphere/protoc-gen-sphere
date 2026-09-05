package parser

import (
	"fmt"
	"net/http"
	"strings"

	validatepb "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

type SwagParams struct {
	Method string
	Path   string
	Auth   string

	PathVars   []ParamsField
	QueryVars  []ParamsField
	FormVars   []ParamsField
	HeaderVars []ParamsField

	Body         string
	ResponseBody string

	DataResponse  string
	ErrorResponse string

	// Stream marks a server-streaming (SSE) method: the response is a
	// text/event-stream of reply messages, not a DataResponse envelope.
	Stream bool
}

var NoBodyMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodDelete:  {},
	http.MethodOptions: {},
}

func BuildAnnotations(g *GeneratedFile, m *protogen.Method, config *SwagParams) (string, error) {
	var builder strings.Builder
	builder.WriteString("// @Summary " + string(m.Desc.Name()) + "\n")
	if desc := swaggerDescription(string(m.Comments.Leading)); desc != "" {
		builder.WriteString("// @Description " + desc + "\n")
	}

	pkgName := string(m.Parent.Desc.ParentFile().Package())
	builder.WriteString("// @Tags " + strings.Join([]string{
		pkgName,
		pkgName + "." + string(m.Parent.Desc.Name()),
	}, ",") + "\n")

	if len(config.FormVars) > 0 {
		builder.WriteString("// @Accept mpfd\n")
	} else {
		builder.WriteString("// @Accept json\n")
	}
	if config.Stream {
		builder.WriteString("// @Produce text/event-stream\n")
	} else {
		builder.WriteString("// @Produce json\n")
	}

	// Add authentication if specified
	if config.Auth != "" {
		builder.WriteString(config.Auth + "\n")
	}

	// Add header parameters
	for _, param := range config.HeaderVars {
		paramType := ProtoTypeToSwaggerType(g, param.Field)
		required := isFieldRequired(param.Field, false)
		_, _ = fmt.Fprintf(&builder, "// @Param %s header %s %v \"%s\"\n", param.Name, paramType, required, param.Name)
	}

	// Add path parameters
	for _, param := range config.PathVars {
		paramType := ProtoTypeToSwaggerType(g, param.Field)
		required := isFieldRequired(param.Field, true)
		_, _ = fmt.Fprintf(&builder, "// @Param %s path %s %v \"%s\"\n", param.Name, paramType, required, param.Name)
	}
	// Add query parameters
	for _, param := range config.QueryVars {
		paramType := ProtoTypeToSwaggerType(g, param.Field)
		required := isFieldRequired(param.Field, false)
		_, _ = fmt.Fprintf(&builder, "// @Param %s query %s %v \"%s\"\n", param.Name, paramType, required, param.Name)
	}
	// Add form parameters
	for _, param := range config.FormVars {
		paramType := ProtoTypeToSwaggerType(g, param.Field)
		required := isFieldRequired(param.Field, false)
		_, _ = fmt.Fprintf(&builder, "// @Param %s formData %s %v \"%s\"\n", param.Name, paramType, required, param.Name)
	}
	// Add a request body. Skip it when the request carries form parameters:
	// in OpenAPI 2.0 `body` and `formData` parameters are mutually exclusive,
	// and a form-bound request has no JSON body to decode.
	_, noBody := NoBodyMethods[config.Method]
	if !noBody && len(config.FormVars) == 0 {
		bodyType, err := buildSwaggerParamTypeByPath(g, m, m.Input, config.Body)
		if err != nil {
			return "", err
		}
		builder.WriteString("// @Param request body " + bodyType + " true \"request body\"\n")
	}

	// Add a response body
	responseType, err := buildSwaggerParamTypeByPath(g, m, m.Output, config.ResponseBody)
	if err != nil {
		return "", err
	}
	if config.Stream {
		// Swagger has no event-stream concept; document the per-event message
		// type instead of pretending there is a DataResponse envelope.
		builder.WriteString("// @Success 200 {object} " + responseType + " \"server-sent events stream of this message; terminated by a done or error event\"\n")
	} else {
		builder.WriteString("// @Success 200 {object} " + config.DataResponse + "[" + responseType + "]\n")
	}
	builder.WriteString("// @Failure 400,401,403,500,default {object} " + config.ErrorResponse + "\n")

	builder.WriteString("// @Router " + config.Path + " [" + strings.ToLower(config.Method) + "]\n")

	return strings.TrimSpace(builder.String()), nil
}

func buildSwaggerParamTypeByPath(g *GeneratedFile, m *protogen.Method, message *protogen.Message, path string) (string, error) {
	name := g.QualifiedGoIdent(message.GoIdent)
	if path != "" {
		field := ProtoKeyPathToField(message, strings.Split(path, "."))
		if field == nil {
			return "", fmt.Errorf("method `%s.%s` field `%s` not found in message `%s`. File: `%s`",
				m.Parent.Desc.Name(),
				m.Desc.Name(),
				path,
				message.Desc.Name(),
				m.Parent.Location.SourceFile,
			)
		} else {
			name = ProtoTypeToSwaggerType(g, field)
		}
	}
	return name, nil
}

// swaggerDescription collapses a method's leading proto comment into a single
// comma-separated line suitable for a Swagger @Description. Empty input yields
// an empty string.
func swaggerDescription(leading string) string {
	desc := strings.TrimSpace(leading)
	if desc == "" {
		return ""
	}
	return strings.TrimSpace(strings.Join(strings.Split(desc, "\n"), ","))
}

func isFieldRequired(field *protogen.Field, defaultRequired bool) bool {
	// If field has optional keyword, it's not required
	if field.Desc.HasOptionalKeyword() {
		return false
	}

	// Check buf.validate required constraint
	opts := field.Desc.Options()
	if opts != nil && proto.HasExtension(opts, validatepb.E_Field) {
		fieldConstraints := proto.GetExtension(opts, validatepb.E_Field).(*validatepb.FieldRules)
		if fieldConstraints != nil {
			return fieldConstraints.GetRequired()
		}
	}

	// Return default value based on parameter type
	// Path parameters default to required (true)
	// Query/Header parameters default to optional (false)
	return defaultRequired
}
