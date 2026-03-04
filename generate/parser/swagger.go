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
	HeaderVars []ParamsField

	Body         string
	ResponseBody string

	DataResponse  string
	ErrorResponse string
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
	desc := strings.TrimSpace(string(m.Comments.Leading))
	if desc != "" {
		desc = strings.TrimSpace(strings.Join(strings.Split(desc, "\n"), ","))
		builder.WriteString("// @Description " + desc + "\n")
	}

	pkgName := string(m.Parent.Desc.ParentFile().Package())
	builder.WriteString("// @Tags " + strings.Join([]string{
		pkgName,
		pkgName + "." + string(m.Parent.Desc.Name()),
	}, ",") + "\n")

	builder.WriteString("// @Accept json\n")
	builder.WriteString("// @Produce json\n")

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
	// Add a request body
	if _, ok := NoBodyMethods[config.Method]; !ok {
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
	builder.WriteString("// @Success 200 {object} " + config.DataResponse + "[" + responseType + "]\n")
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
