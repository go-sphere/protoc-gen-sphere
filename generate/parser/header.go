package parser

import (
	bindingpb "github.com/go-sphere/binding/sphere/binding"
	"google.golang.org/protobuf/compiler/protogen"
)

type HeaderField struct {
	Name  string
	Field *protogen.Field
}

func GinHeaderParams(m *protogen.Method) ([]HeaderField, error) {
	var fields []HeaderField
	for _, field := range m.Input.Fields {
		name := string(field.Desc.Name())
		if checkBindingLocation(m.Input, field, bindingpb.BindingLocation_BINDING_LOCATION_HEADER) {
			fields = append(fields, HeaderField{
				Name:  name,
				Field: field,
			})
		}
	}
	return fields, nil
}
