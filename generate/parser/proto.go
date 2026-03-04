package parser

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type GeneratedFile struct {
	g       *protogen.GeneratedFile
	dummies map[protogen.GoImportPath]protogen.GoIdent
}

func NewGen(g *protogen.GeneratedFile) *GeneratedFile {
	return &GeneratedFile{
		g:       g,
		dummies: make(map[protogen.GoImportPath]protogen.GoIdent),
	}
}

func (g *GeneratedFile) QualifiedGoIdent(id protogen.GoIdent) string {
	if _, ok := g.dummies[id.GoImportPath]; !ok {
		g.dummies[id.GoImportPath] = id
	}
	return g.g.QualifiedGoIdent(id)
}

func (g *GeneratedFile) Dummies() map[protogen.GoImportPath]protogen.GoIdent {
	return g.dummies
}

func ProtoKeyPathToField(message *protogen.Message, keypath []string) *protogen.Field {
	if len(keypath) == 0 || message == nil {
		return nil
	}

	for _, field := range message.Fields {
		if string(field.Desc.Name()) != keypath[0] {
			continue
		}

		if len(keypath) == 1 {
			return field
		}

		if field.Message != nil {
			return ProtoKeyPathToField(field.Message, keypath[1:])
		}

		if field.Oneof != nil {
			for _, oneofField := range field.Oneof.Fields {
				if string(oneofField.Desc.Name()) == keypath[1] {
					if len(keypath) == 2 {
						return oneofField
					}
					if len(keypath) > 2 && oneofField.Message != nil {
						return ProtoKeyPathToField(oneofField.Message, keypath[2:])
					}
				}
			}
		}
	}
	return nil
}

func ProtoKeyPathToGoKeyPath(message *protogen.Message, keypath []string) []string {
	if len(keypath) == 0 || message == nil {
		return nil
	}
	goKeyPath := make([]string, 0, len(keypath))
	for _, key := range keypath {
		field := ProtoKeyPathToField(message, []string{key})
		if field == nil {
			return nil
		}
		goKeyPath = append(goKeyPath, field.GoName)
		message = field.Message
	}
	return goKeyPath
}

func ProtoTypeToGoType(g *GeneratedFile, field *protogen.Field, usePtrForMessage bool) string {
	switch {
	case field.Desc.IsMap():
		// For map, get key and value types
		keyField := field.Message.Fields[0]
		valField := field.Message.Fields[1]
		key := singularProtoTypeToGoType(g, keyField, false) // map keys are never pointers

		// Handle map value - could be message, list, or scalar
		var val string
		if valField.Desc.IsList() {
			// map value is an array: map[string][]Type
			elemType := singularProtoTypeToGoType(g, valField, usePtrForMessage)
			val = fmt.Sprintf("[]%s", elemType)
		} else {
			val = singularProtoTypeToGoType(g, valField, usePtrForMessage)
		}
		return fmt.Sprintf("map[%s]%s", key, val)
	case field.Desc.IsList():
		// For repeated fields, always use pointer for message types
		elemType := singularProtoTypeToGoType(g, field, true)
		return fmt.Sprintf("[]%s", elemType)
	default:
		return singularProtoTypeToGoType(g, field, usePtrForMessage)
	}
}

func singularProtoTypeToGoType(g *GeneratedFile, field *protogen.Field, usePtrForMessage bool) string {
	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		return "bool"
	case protoreflect.Int32Kind:
		return "int32"
	case protoreflect.Sint32Kind:
		return "int32"
	case protoreflect.Uint32Kind:
		return "uint32"
	case protoreflect.Int64Kind:
		return "int64"
	case protoreflect.Sint64Kind:
		return "int64"
	case protoreflect.Uint64Kind:
		return "uint64"
	case protoreflect.Sfixed32Kind:
		return "int32"
	case protoreflect.Fixed32Kind:
		return "uint32"
	case protoreflect.Sfixed64Kind:
		return "int64"
	case protoreflect.Fixed64Kind:
		return "uint64"
	case protoreflect.FloatKind:
		return "float32"
	case protoreflect.DoubleKind:
		return "float64"
	case protoreflect.StringKind:
		return "string"
	case protoreflect.BytesKind:
		return "[]byte"
	case protoreflect.EnumKind:
		if field.Enum != nil {
			return g.QualifiedGoIdent(field.Enum.GoIdent)
		}
		return "int32" // Fallback for unknown enum types
	case protoreflect.MessageKind:
		if field.Message != nil {
			ident := g.QualifiedGoIdent(field.Message.GoIdent)
			if usePtrForMessage {
				return "*" + ident
			}
			return ident
		}
		return "any" // Fallback for unknown message types
	default:
		return "any" // Fallback for unknown types
	}
}

func ProtoTypeToSwaggerType(g *GeneratedFile, field *protogen.Field) string {
	switch {
	case field.Desc.IsMap():
		key := singularSwaggerParamType(g, field.Message.Fields[0])
		val := singularSwaggerParamType(g, field.Message.Fields[1])
		return fmt.Sprintf("map[%s]%s", key, val)
	case field.Desc.IsList():
		elemType := singularSwaggerParamType(g, field)
		return fmt.Sprintf("[]%s", elemType)
	default:
		return singularSwaggerParamType(g, field)
	}
}

func singularSwaggerParamType(g *GeneratedFile, field *protogen.Field) string {
	switch field.Desc.Kind() {
	case protoreflect.BoolKind:
		return "boolean"
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Uint32Kind,
		protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Uint64Kind,
		protoreflect.Sfixed32Kind, protoreflect.Fixed32Kind,
		protoreflect.Sfixed64Kind, protoreflect.Fixed64Kind:
		return "integer"
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return "number"
	case protoreflect.StringKind:
		return "string"
	case protoreflect.BytesKind:
		return "string" // Swagger doesn't have a specific type for bytes, so we use string
	case protoreflect.EnumKind:
		if field.Enum != nil {
			return g.QualifiedGoIdent(field.Enum.GoIdent)
		}
		return "integer"
	case protoreflect.MessageKind:
		if field.Message != nil {
			return g.QualifiedGoIdent(field.Message.GoIdent)
		}
		return "any"
	default:
		return "any"
	}
}
