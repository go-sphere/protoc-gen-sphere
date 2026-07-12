package parser

import (
	"fmt"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ProtoTypeToGoType returns the Go type expression for a proto field, handling
// maps, repeated fields and scalars. usePtrForMessage controls whether singular
// message types are rendered as pointers.
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

// ProtoTypeToSwaggerType returns the Swagger type expression for a proto field,
// handling maps, repeated fields and scalars.
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
			// Well-known types (Timestamp/Duration/wrapperspb.*Value) have a
			// canonical scalar JSON representation. Emitting the Go package type
			// (e.g. timestamppb.Timestamp) would produce a bogus Swagger schema
			// reference, so map them to their scalar form the way grpc-gateway
			// does.
			if swaggerType, ok := wellKnownSwaggerScalar(field); ok {
				return swaggerType
			}
			return g.QualifiedGoIdent(field.Message.GoIdent)
		}
		return "any"
	default:
		return "any"
	}
}

// wellKnownSwaggerScalar maps the well-known message types that carry a natural
// scalar representation to their Swagger scalar type. It returns ok=false for
// any other message type. The set mirrors grpc-gateway: Timestamp/Duration are
// rendered as strings and the wrapperspb.*Value types collapse to their inner
// scalar.
func wellKnownSwaggerScalar(field *protogen.Field) (string, bool) {
	if field.Message == nil {
		return "", false
	}
	switch field.Message.Desc.FullName() {
	case "google.protobuf.Timestamp", "google.protobuf.Duration",
		"google.protobuf.StringValue", "google.protobuf.BytesValue":
		return "string", true
	case "google.protobuf.BoolValue":
		return "boolean", true
	case "google.protobuf.DoubleValue", "google.protobuf.FloatValue":
		return "number", true
	case "google.protobuf.Int32Value", "google.protobuf.UInt32Value",
		"google.protobuf.Int64Value", "google.protobuf.UInt64Value":
		return "integer", true
	}
	return "", false
}
