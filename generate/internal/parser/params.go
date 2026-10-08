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
		case bindingpb.BindingLocation_BINDING_LOCATION_HEADER:
			continue
		case bindingpb.BindingLocation_BINDING_LOCATION_FORM:
			// A body-less request carries no form payload, and the adapters
			// disagree on whether BindForm falls back to the query string.
			if _, ok := NoBodyMethods[method]; ok {
				return nil, fmt.Errorf("method `%s.%s` field `%s` is bound to FORM, which is not allowed on %s: the request has no body, and not every httpx adapter reads form values from the query string; bind it to QUERY instead. File: `%s`, Field: `%s`",
					m.Parent.Desc.Name(),
					m.Desc.Name(),
					name,
					method,
					m.Parent.Location.SourceFile,
					m.Input.Desc.Name(),
				)
			}
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
// oneof. Requests containing one are rejected before binding (see
// FindRequestOneof); the collectors still skip such fields so they never emit
// a binding for one.
func isRealOneofMember(field *protogen.Field) bool {
	return field.Oneof != nil && !field.Oneof.Desc.IsSynthetic()
}

// RequestOneof is a real oneof found in a request message tree.
type RequestOneof struct {
	// Path is the dot-separated proto field path from the request message to
	// the message declaring the oneof; empty when the request message itself
	// declares it. Map fields contribute their own name only.
	Path string
	// Oneof is the offending oneof.
	Oneof *protogen.Oneof
}

// FindRequestOneof returns the first real (non-synthetic) oneof declared by
// message or by a message reachable through its fields, including map values,
// searching depth-first in declaration order. Messages of the google.protobuf
// package are not searched. ok is false when there is none.
//
// No generated handler can bind a real oneof: the JSON body is decoded with
// encoding/json, which ignores the interface-typed oneof field protoc-gen-go
// emits, and the query/uri/header/form binders skip oneof members.
func FindRequestOneof(message *protogen.Message) (RequestOneof, bool) {
	return findOneof(message, "", make(map[protoreflect.FullName]struct{}))
}

func findOneof(message *protogen.Message, path string, seen map[protoreflect.FullName]struct{}) (RequestOneof, bool) {
	if message == nil || message.Desc.ParentFile().Package() == "google.protobuf" {
		return RequestOneof{}, false
	}
	if _, ok := seen[message.Desc.FullName()]; ok {
		return RequestOneof{}, false
	}
	seen[message.Desc.FullName()] = struct{}{}
	for _, oneof := range message.Oneofs {
		if !oneof.Desc.IsSynthetic() {
			return RequestOneof{Path: path, Oneof: oneof}, true
		}
	}
	for _, field := range message.Fields {
		next := field.Message
		if field.Desc.IsMap() {
			next = field.Message.Fields[1].Message
		}
		fieldPath := string(field.Desc.Name())
		if path != "" {
			fieldPath = path + "." + fieldPath
		}
		if found, ok := findOneof(next, fieldPath, seen); ok {
			return found, true
		}
	}
	return RequestOneof{}, false
}

// JSONFields returns the top-level fields of message whose binding location is
// JSON, explicitly or by default (no location and no default_location). Real
// oneof members are excluded.
func JSONFields(message *protogen.Message) []*protogen.Field {
	var fields []*protogen.Field
	for _, field := range message.Fields {
		if isRealOneofMember(field) {
			continue
		}
		switch bindingLocationOf(message, field) {
		case bindingpb.BindingLocation_BINDING_LOCATION_UNSPECIFIED,
			bindingpb.BindingLocation_BINDING_LOCATION_JSON:
			fields = append(fields, field)
		}
	}
	return fields
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
// from a single QUERY/URI/HEADER token. Maps, bytes and message fields
// (well-known types included) have no scalar text form the runtime binders
// decode, so binding them silently produces a tag the runtime cannot satisfy;
// failing at generation time surfaces the mistake early.
func checkScalarBindable(m *protogen.Method, field *protogen.Field, location string) error {
	if isScalarBindable(field) {
		return nil
	}
	return fmt.Errorf("method `%s.%s` field `%s` of type `%s` cannot be bound to %s: only scalar and enum types are supported there. File: `%s`, Message: `%s`",
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
// (query/uri/header). Maps, bytes and messages cannot: the form decoders the
// httpx adapters use have no conversion for well-known types such as
// Timestamp, Duration or wrapperspb.*Value either.
//
// isScalarBindable and fieldKindDesc are mirrored by hand in
// protoc-gen-sphere-binding (generate/binding/tagger.go). The shared fixture
// generate/http/testdata/proto/scalar_bindability.proto and its expected table
// testdata/golden/scalar_bindability.golden pin the decisions in both repos
// (TestScalarBindabilityContract). Keep them byte-identical; update both.
func isScalarBindable(field *protogen.Field) bool {
	if field.Desc.IsMap() {
		return false
	}
	switch field.Desc.Kind() {
	case protoreflect.BytesKind, protoreflect.MessageKind, protoreflect.GroupKind:
		return false
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
