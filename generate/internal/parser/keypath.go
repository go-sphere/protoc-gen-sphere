package parser

import "google.golang.org/protobuf/compiler/protogen"

// ProtoKeyPathToField resolves a proto field key path (e.g. ["data", "id"]) to
// the terminal *protogen.Field, descending through nested messages and oneofs.
// It returns nil when any segment of the path cannot be found.
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

// ProtoKeyPathToGoKeyPath translates a proto field key path into the equivalent
// Go field names (e.g. ["data", "id"] -> ["Data", "Id"]). It returns nil when
// any segment cannot be resolved.
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
