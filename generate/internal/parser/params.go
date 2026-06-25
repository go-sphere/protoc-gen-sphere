package parser

import (
	"fmt"

	bindingpb "github.com/go-sphere/binding/sphere/binding"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
)

type ParamsField struct {
	Name     string
	Wildcard bool
	Field    *protogen.Field
}

func HeaderParams(m *protogen.Method) ([]ParamsField, error) {
	var fields []ParamsField
	for _, field := range m.Input.Fields {
		name := string(field.Desc.Name())
		if checkBindingLocation(m.Input, field, bindingpb.BindingLocation_BINDING_LOCATION_HEADER) {
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
	// :param
	namedMatches := namedParamRegex.FindAllStringSubmatch(route, -1)
	for _, match := range namedMatches {
		if len(match) > 1 {
			params[match[1]] = false
		}
	}
	// *param
	wildcardMatches := wildcardParamRegex.FindAllStringSubmatch(route, -1)
	for _, match := range wildcardMatches {
		if len(match) > 1 {
			params[match[1]] = true
		}
	}

	for _, field := range m.Input.Fields {
		name := string(field.Desc.Name())
		wildcard, exist := params[name]
		if exist {
			if checkBindingLocation(m.Input, field, bindingpb.BindingLocation_BINDING_LOCATION_URI) {
				fields = append(fields, ParamsField{
					Name:     name,
					Wildcard: wildcard,
					Field:    field,
				})
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
	return fields, nil
}

func QueryParams(m *protogen.Method, method string, pathVars []ParamsField) ([]ParamsField, error) {
	var fields []ParamsField
	params := make(map[string]struct{}, len(pathVars))
	for _, v := range pathVars {
		params[v.Name] = struct{}{}
	}
	for _, field := range m.Input.Fields {
		name := string(field.Desc.Name())
		if _, ok := params[name]; ok {
			continue
		}
		if checkBindingLocation(m.Input, field, bindingpb.BindingLocation_BINDING_LOCATION_QUERY) {
			fields = append(fields, ParamsField{
				Name:  name,
				Field: field,
			})
		} else {
			if _, ok := NoBodyMethods[method]; ok {
				return nil, fmt.Errorf("method `%s.%s` parameter `%s` is not bound to either query or uri. File: `%s`, Field: `%s`",
					m.Parent.Desc.Name(),
					m.Desc.Name(),
					name,
					m.Parent.Location.SourceFile,
					m.Input.Desc.Name(),
				)
			}
		}
	}
	return fields, nil
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
