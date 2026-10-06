package parser

import (
	"fmt"

	bindingpb "github.com/go-sphere/binding/sphere/binding"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ParamsField struct {
	Name     string
	Wildcard bool
	Field    *protogen.Field
}

func HeaderParams(m *protogen.Method) ([]ParamsField, error) {
	var fields []ParamsField
	for _, field := range m.Input.Fields {
		if isRealOneofMember(field) {
			continue
		}
		name := string(field.Desc.Name())
		if checkBindingLocation(m.Input, field, bindingpb.BindingLocation_BINDING_LOCATION_HEADER) {
			if err := checkScalarBindable(m, field, "HEADER"); err != nil {
				return nil, err
			}
			fields = append(fields, ParamsField{
				Name:  name,
				Field: field,
			})
		}
	}
	return fields, nil
}

func URIParams(m *protogen.Method, route string) ([]ParamsField, error) {
	var fields []ParamsField

	params := make(map[string]bool)
	// :param (only at segment start; a mid-segment colon is a literal)
	namedMatches := namedParamRegex.FindAllStringSubmatch(route, -1)
	for _, match := range namedMatches {
		if len(match) > 2 {
			params[match[2]] = false
		}
	}
	// *param
	wildcardMatches := wildcardParamRegex.FindAllStringSubmatch(route, -1)
	for _, match := range wildcardMatches {
		if len(match) > 2 {
			params[match[2]] = true
		}
	}

	matched := make(map[string]struct{}, len(params))
	for _, field := range m.Input.Fields {
		if isRealOneofMember(field) {
			continue
		}
		name := string(field.Desc.Name())
		key, wildcard, exist := routeParamForField(name, params)
		if exist {
			if checkBindingLocation(m.Input, field, bindingpb.BindingLocation_BINDING_LOCATION_URI) {
				if err := checkScalarBindable(m, field, "URI"); err != nil {
					return nil, err
				}
				fields = append(fields, ParamsField{
					Name:     name,
					Wildcard: wildcard,
					Field:    field,
				})
				matched[key] = struct{}{}
			} else {
				return nil, fmt.Errorf("method `%s.%s` parameter `%s` is not bound to URI, but it is used in route `%s`. File: `%s`, Field: `%s`",
					m.Parent.Desc.Name(),
					m.Desc.Name(),
					name,
					route,
					m.Parent.Location.SourceFile,
					m.Input.Desc.Name(),
				)
			}
		}
	}
	for param := range params {
		if _, ok := matched[param]; ok {
			continue
		}
		return nil, fmt.Errorf("method `%s.%s` route `%s` has parameter `%s` that does not match a top-level request field. Nested path variables such as {user.id} are not supported; declare a top-level field and mark it BINDING_LOCATION_URI. File: `%s`, Message: `%s`",
			m.Parent.Desc.Name(),
			m.Desc.Name(),
			route,
			param,
			m.Parent.Location.SourceFile,
			m.Input.Desc.Name(),
		)
	}
	return fields, nil
}

func QueryParams(m *protogen.Method, method string, pathVars []ParamsField) ([]ParamsField, error) {
	var fields []ParamsField
	params := make(map[string]struct{}, len(pathVars))
	for _, v := range pathVars {
		params[v.Name] = struct{}{}
	}
	for _, field := range m.Input.Fields {
		if isRealOneofMember(field) {
			continue
		}
		name := string(field.Desc.Name())
		if _, ok := params[name]; ok {
			continue
		}
		loc := bindingLocationOf(m.Input, field)
		switch loc {
		case bindingpb.BindingLocation_BINDING_LOCATION_HEADER,
			bindingpb.BindingLocation_BINDING_LOCATION_FORM:
			continue
		case bindingpb.BindingLocation_BINDING_LOCATION_JSON:
			if _, ok := NoBodyMethods[method]; ok {
				return nil, fmt.Errorf("method `%s.%s` field `%s` is bound to JSON, which is not allowed on %s. File: `%s`, Field: `%s`",
					m.Parent.Desc.Name(),
					m.Desc.Name(),
					name,
					method,
					m.Parent.Location.SourceFile,
					m.Input.Desc.Name(),
				)
			}
			continue
		}
		if checkBindingLocation(m.Input, field, bindingpb.BindingLocation_BINDING_LOCATION_QUERY) {
			if err := checkScalarBindable(m, field, "QUERY"); err != nil {
				return nil, err
			}
			fields = append(fields, ParamsField{
				Name:  name,
				Field: field,
			})
		} else if _, ok := NoBodyMethods[method]; ok {
			return nil, fmt.Errorf("method `%s.%s` parameter `%s` is not bound to query, uri, header, or form. File: `%s`, Field: `%s`",
				m.Parent.Desc.Name(),
				m.Desc.Name(),
				name,
				m.Parent.Location.SourceFile,
				m.Input.Desc.Name(),
			)
		}
	}
	return fields, nil
}

func FormParams(m *protogen.Method) ([]ParamsField, error) {
	var fields []ParamsField
	for _, field := range m.Input.Fields {
		if isRealOneofMember(field) {
			continue
		}
		name := string(field.Desc.Name())
		if checkBindingLocation(m.Input, field, bindingpb.BindingLocation_BINDING_LOCATION_FORM) {
			fields = append(fields, ParamsField{
				Name:  name,
				Field: field,
			})
		}
	}
	return fields, nil
}

// isRealOneofMember reports whether field belongs to a real (non-synthetic)
// oneof. Such fields bind only through the JSON body: every query/uri/header/
// form collector skips them (see OneofBindingIssues).
func isRealOneofMember(field *protogen.Field) bool {
	return field.Oneof != nil && !field.Oneof.Desc.IsSynthetic()
}

// OneofBindingIssue is a binding location declared on a real oneof, or on one
// of its member fields, that the generator cannot honor.
type OneofBindingIssue struct {
	// Subject names the declaring element, e.g. "oneof `kind`" or
	// "field `text`".
	Subject  string
	Location bindingpb.BindingLocation
}

// OneofBindingIssues returns the non-JSON binding locations declared on
// message's real oneofs (default_oneof_location) and their members
// (location), in declaration order. Oneof fields bind only via the JSON body,
// so such declarations are silently ignored by the query/uri/header/form
// collectors; callers report them as warnings rather than errors to keep
// existing protos generating. An explicit UNSPECIFIED or JSON declaration is
// consistent with that contract and is not reported.
func OneofBindingIssues(message *protogen.Message) []OneofBindingIssue {
	var issues []OneofBindingIssue
	add := func(subject string, opts proto.Message, ext protoreflect.ExtensionType) {
		if !proto.HasExtension(opts, ext) {
			return
		}
		loc, ok := proto.GetExtension(opts, ext).(bindingpb.BindingLocation)
		if !ok || loc == bindingpb.BindingLocation_BINDING_LOCATION_UNSPECIFIED || loc == bindingpb.BindingLocation_BINDING_LOCATION_JSON {
			return
		}
		issues = append(issues, OneofBindingIssue{Subject: subject, Location: loc})
	}
	for _, oneof := range message.Oneofs {
		if oneof.Desc.IsSynthetic() {
			continue
		}
		add(fmt.Sprintf("oneof `%s`", oneof.Desc.Name()), oneof.Desc.Options(), bindingpb.E_DefaultOneofLocation)
		for _, field := range oneof.Fields {
			add(fmt.Sprintf("field `%s`", field.Desc.Name()), field.Desc.Options(), bindingpb.E_Location)
		}
	}
	return issues
}

func routeParamForField(fieldName string, params map[string]bool) (string, bool, bool) {
	if wildcard, ok := params[fieldName]; ok {
		return fieldName, wildcard, true
	}
	cleaned := cleanParamName(fieldName)
	if cleaned != fieldName {
		if wildcard, ok := params[cleaned]; ok {
			return cleaned, wildcard, true
		}
	}
	return "", false, false
}

func bindingLocationOf(message *protogen.Message, field *protogen.Field) bindingpb.BindingLocation {
	if proto.HasExtension(field.Desc.Options(), bindingpb.E_Location) {
		return proto.GetExtension(field.Desc.Options(), bindingpb.E_Location).(bindingpb.BindingLocation)
	}
	if proto.HasExtension(message.Desc.Options(), bindingpb.E_DefaultLocation) {
		return proto.GetExtension(message.Desc.Options(), bindingpb.E_DefaultLocation).(bindingpb.BindingLocation)
	}
	return bindingpb.BindingLocation_BINDING_LOCATION_UNSPECIFIED
}

// checkScalarBindable returns a descriptive error when field cannot be bound
// from a single QUERY/URI/HEADER token. Maps, bytes and arbitrary messages have
// no scalar text form, so binding them silently produces a tag the runtime
// cannot decode; failing at generation time surfaces the mistake early. Scalar
// kinds and well-known scalar wrappers (Timestamp/Duration/wrapperspb.*Value)
// are allowed through.
func checkScalarBindable(m *protogen.Method, field *protogen.Field, location string) error {
	if isScalarBindable(field) {
		return nil
	}
	return fmt.Errorf("method `%s.%s` field `%s` of type `%s` cannot be bound to %s: only scalar types (and well-known scalar wrappers) are supported there. File: `%s`, Message: `%s`",
		m.Parent.Desc.Name(),
		m.Desc.Name(),
		field.Desc.Name(),
		fieldKindDesc(field),
		location,
		m.Parent.Location.SourceFile,
		m.Input.Desc.Name(),
	)
}

// isScalarBindable reports whether field can be bound from a single string token
// (query/uri/header). Maps and bytes cannot; message fields are only allowed
// when they are well-known scalar wrappers.
//
// isScalarBindable, fieldKindDesc and the wellKnownSwaggerScalar allowlist are
// mirrored by hand in protoc-gen-sphere-binding (generate/binding/tagger.go).
// The shared fixture generate/http/testdata/proto/scalar_bindability.proto and
// its expected table testdata/golden/scalar_bindability.golden pin the
// decisions in both repos (TestScalarBindabilityContract). Keep them
// byte-identical; update both.
func isScalarBindable(field *protogen.Field) bool {
	if field.Desc.IsMap() {
		return false
	}
	switch field.Desc.Kind() {
	case protoreflect.BytesKind:
		return false
	case protoreflect.MessageKind, protoreflect.GroupKind:
		_, ok := wellKnownSwaggerScalar(field)
		return ok
	default:
		return true
	}
}

// fieldKindDesc returns a human-readable description of a field's type for use
// in error messages (e.g. "map", "bytes", "message").
func fieldKindDesc(field *protogen.Field) string {
	switch {
	case field.Desc.IsMap():
		return "map"
	case field.Desc.Kind() == protoreflect.BytesKind:
		return "bytes"
	case field.Desc.Kind() == protoreflect.MessageKind || field.Desc.Kind() == protoreflect.GroupKind:
		return "message"
	default:
		return field.Desc.Kind().String()
	}
}

func checkBindingLocation(message *protogen.Message, field *protogen.Field, location bindingpb.BindingLocation) bool {
	if proto.HasExtension(field.Desc.Options(), bindingpb.E_Location) {
		bindingLocation := proto.GetExtension(field.Desc.Options(), bindingpb.E_Location).(bindingpb.BindingLocation)
		return bindingLocation == location
	}
	if proto.HasExtension(message.Desc.Options(), bindingpb.E_DefaultLocation) {
		defaultBindingLocation := proto.GetExtension(message.Desc.Options(), bindingpb.E_DefaultLocation).(bindingpb.BindingLocation)
		return defaultBindingLocation == location
	}
	return false
}
