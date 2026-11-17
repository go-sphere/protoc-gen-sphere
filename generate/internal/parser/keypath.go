package parser

import "google.golang.org/protobuf/compiler/protogen"

// FindField returns the direct top-level field of message matching proto name,
// or nil when message is nil or no field matches.
func FindField(message *protogen.Message, name string) *protogen.Field {
	if message == nil || name == "" {
		return nil
	}
	for _, field := range message.Fields {
		if string(field.Desc.Name()) == name {
			return field
		}
	}
	return nil
}
