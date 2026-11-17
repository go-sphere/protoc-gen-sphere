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

	Body         *protogen.Field
	ResponseBody *protogen.Field

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
	op := buildOperation(g, m, config)
	block, err := op.Render()
	if err != nil {
		return "", fmt.Errorf("method `%s.%s`: %w",
			m.Parent.Desc.Name(), m.Desc.Name(), err)
	}
	return block, nil
}

func buildOperation(g *GeneratedFile, m *protogen.Method, config *SwagParams) *swagspec.Operation {
	pkgName := string(m.Parent.Desc.ParentFile().Package())
	svcName := string(m.Parent.Desc.Name())
	tags := []string{pkgName, pkgName + "." + svcName}
	if pkgName == "" {
		tags = []string{svcName}
	}
	op := &swagspec.Operation{
		Summary:      string(m.Desc.Name()),
		Tags:         tags,
		Accept:       "json",
		Produce:      "json",
		RouterPath:   config.Path,
		RouterMethod: strings.ToLower(config.Method),
	}
	if desc := swaggerDescription(string(m.Comments.Leading)); desc != "" {
		op.Description = desc
	}
	if strings.TrimSpace(config.Auth) != "" {
		auth := strings.ReplaceAll(config.Auth, "\r\n", "\n")
		auth = strings.ReplaceAll(auth, "\r", "")
		op.RawLines = strings.Split(strings.TrimRight(auth, "\n"), "\n")
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
			Required: true,
			// OpenAPI 2.0 requires path parameters to be mandatory and the
			// router cannot match the route without them anyway.
			Description: param.Name,
		})
	}
	addParams(config.QueryVars, swagspec.LocationQuery)

	// QueryParams rejects form fields on GET/HEAD/DELETE/OPTIONS, so form
	// fields only reach here on body-carrying methods.
	noBody := isNoBodyMethod(config.Method)
	addParams(config.FormVars, swagspec.LocationFormData)

	// Add the request body. It exists only when the rule declares one:
	// form-bound requests have no JSON body (OpenAPI 2.0 makes body and
	// formData mutually exclusive), and no-body methods decode nothing.
	if config.HasBody && !noBody && len(config.FormVars) == 0 {
		bodyType := swaggerMessageOrFieldType(g, m.Input, config.Body)
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
	responseType := swaggerMessageOrFieldType(g, m.Output, config.ResponseBody)
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
	return op
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

func swaggerMessageOrFieldType(g *GeneratedFile, message *protogen.Message, field *protogen.Field) string {
	if field != nil {
		return ProtoTypeToSwaggerType(g, field)
	}
	return g.QualifiedGoIdent(message.GoIdent)
}

// swaggerDescription collapses a method's leading proto comment into a single
// comma-separated line suitable for a Swagger @Description. Empty input yields
// an empty string.
func swaggerDescription(leading string) string {
	desc := strings.ReplaceAll(leading, "\r\n", "\n")
	desc = strings.ReplaceAll(desc, "\r", "")
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	return strings.TrimSpace(strings.Join(strings.Split(desc, "\n"), ","))
}

func isFieldRequired(field *protogen.Field, defaultRequired bool) bool {
	// Check buf.validate required constraint first so an explicit required=true
	// rule overrides proto3 optional and defaultRequired.
	opts := field.Desc.Options()
	if opts != nil && proto.HasExtension(opts, validatepb.E_Field) {
		fieldConstraints := proto.GetExtension(opts, validatepb.E_Field).(*validatepb.FieldRules)
		if fieldConstraints != nil && fieldConstraints.GetRequired() {
			return true
		}
	}

	// If field has optional keyword, it's not required
	if field.Desc.HasOptionalKeyword() {
		return false
	}

	// Return default value based on parameter type
	// Path parameters default to required (true)
	// Query/Header parameters default to optional (false)
	return defaultRequired
}
