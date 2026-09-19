package parser

import (
	"fmt"
	"net/http"
	"strings"

	validatepb "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"github.com/go-sphere/protoc-gen-sphere/generate/internal/swagspec"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

// streamSuccessDescription documents the SSE framing of a streaming response,
// since Swagger itself has no event-stream concept.
const streamSuccessDescription = "server-sent events stream of this message; terminated by a done or error event"

type SwagParams struct {
	Method string
	Path   string
	Auth   string
	// HasBody mirrors the HTTP rule's body declaration: only a rule that
	// declares a body may advertise one in the Swagger docs.
	HasBody bool

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

// BuildAnnotations synthesizes the swag annotation block for one method. It
// builds a swagspec.Operation from the parsed proto facts and delegates both
// validation and rendering to it, so the swag comment syntax has a single
// owner and invalid combinations fail generation instead of producing
// misleading docs.
func BuildAnnotations(g *GeneratedFile, m *protogen.Method, config *SwagParams) (string, error) {
	op, err := buildOperation(g, m, config)
	if err != nil {
		return "", err
	}
	block, err := op.Render()
	if err != nil {
		return "", fmt.Errorf("method `%s.%s`: %w",
			m.Parent.Desc.Name(), m.Desc.Name(), err)
	}
	return block, nil
}

func buildOperation(g *GeneratedFile, m *protogen.Method, config *SwagParams) (*swagspec.Operation, error) {
	pkgName := string(m.Parent.Desc.ParentFile().Package())
	op := &swagspec.Operation{
		Summary: string(m.Desc.Name()),
		Tags: []string{
			pkgName,
			pkgName + "." + string(m.Parent.Desc.Name()),
		},
		Accept:       "json",
		Produce:      "json",
		RouterPath:   config.Path,
		RouterMethod: strings.ToLower(config.Method),
	}
	if desc := swaggerDescription(string(m.Comments.Leading)); desc != "" {
		op.Description = desc
	}
	if strings.TrimSpace(config.Auth) != "" {
		op.RawLines = strings.Split(strings.TrimRight(config.Auth, "\n"), "\n")
	}
	if config.Stream {
		op.Produce = "text/event-stream"
	}

	addParams := func(fields []ParamsField, location swagspec.Location) {
		for _, param := range fields {
			op.Params = append(op.Params, swagspec.Param{
				Name:        param.Name,
				In:          location,
				Type:        ProtoTypeToSwaggerParamType(g, param.Field),
				Required:    isFieldRequired(param.Field, false),
				Description: param.Name,
			})
		}
	}
	addParams(config.HeaderVars, swagspec.LocationHeader)
	for _, param := range config.PathVars {
		op.Params = append(op.Params, swagspec.Param{
			Name:     param.Name,
			In:       swagspec.LocationPath,
			Type:     pathParamSwaggerType(g, param.Field),
			Required: isFieldRequired(param.Field, true),
			// swagspec rejects optional path params: OpenAPI 2.0 requires
			// path parameters to be mandatory and the router cannot match
			// the route without them anyway.
			Description: param.Name,
		})
	}
	addParams(config.QueryVars, swagspec.LocationQuery)

	// Form-bound fields decode from the request payload, which only
	// body-carrying methods have. On GET/HEAD/DELETE/OPTIONS the runtime
	// (gin form binding) reads them from the query string, so document them
	// as query parameters there.
	noBody := isNoBodyMethod(config.Method)
	formLocation := swagspec.LocationFormData
	if noBody {
		formLocation = swagspec.LocationQuery
	}
	addParams(config.FormVars, formLocation)

	// Add the request body. It exists only when the rule declares one:
	// form-bound requests have no JSON body (OpenAPI 2.0 makes body and
	// formData mutually exclusive), and no-body methods decode nothing.
	if config.HasBody && !noBody && len(config.FormVars) == 0 {
		bodyType, err := buildSwaggerParamTypeByPath(g, m, m.Input, config.Body)
		if err != nil {
			return nil, err
		}
		op.Params = append(op.Params, swagspec.Param{
			Name:        "request",
			In:          swagspec.LocationBody,
			Type:        bodyType,
			Required:    true,
			Description: "request body",
		})
	}

	for _, param := range op.Params {
		if param.In == swagspec.LocationFormData {
			op.Accept = "mpfd"
			break
		}
	}

	// Add a response body.
	responseType, err := buildSwaggerParamTypeByPath(g, m, m.Output, config.ResponseBody)
	if err != nil {
		return nil, err
	}
	if config.Stream {
		// Swagger has no event-stream concept; document the per-event message
		// type instead of pretending there is a DataResponse envelope.
		op.Success = swagspec.Response{
			Codes:       []string{"200"},
			Type:        responseType,
			Description: streamSuccessDescription,
		}
	} else {
		op.Success = swagspec.Response{
			Codes: []string{"200"},
			Type:  config.DataResponse + "[" + responseType + "]",
		}
	}
	op.Failure = swagspec.Response{
		Codes: []string{"400", "401", "403", "500", "default"},
		Type:  config.ErrorResponse,
	}
	return op, nil
}

// pathParamSwaggerType renders the Swagger type of a single path token. URI
// variables always arrive as one raw string token, so a repeated field
// collapses to its element type instead of an unbindable array.
func pathParamSwaggerType(g *GeneratedFile, field *protogen.Field) string {
	if field.Desc.IsList() {
		return singularSwaggerParamType(g, field)
	}
	return ProtoTypeToSwaggerType(g, field)
}

func isNoBodyMethod(method string) bool {
	_, ok := NoBodyMethods[method]
	return ok
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
