package http

import (
	validatepb "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// hasValidateOptions reports whether a field carries a buf.validate field rule.
func hasValidateOptions(field *protogen.Field) bool {
	opts := field.Desc.Options().(*descriptorpb.FieldOptions)
	return proto.HasExtension(opts, validatepb.E_Field)
}

// hasValidateOptionsInMessage reports whether a message carries a buf.validate
// message rule.
func hasValidateOptionsInMessage(msg *protogen.Message) bool {
	return proto.HasExtension(msg.Desc.Options(), validatepb.E_Message)
}

// requestNeedsValidate reports whether protovalidate.Validate should run on the
// request. Rules on nested messages (and map-entry values) count; proto
// recursion is guarded by the seen set.
func requestNeedsValidate(msg *protogen.Message) bool {
	return messageHasValidate(msg, make(map[string]struct{}))
}

func messageHasValidate(msg *protogen.Message, seen map[string]struct{}) bool {
	if msg == nil {
		return false
	}
	key := string(msg.Desc.FullName())
	if _, ok := seen[key]; ok {
		return false
	}
	seen[key] = struct{}{}
	if hasValidateOptionsInMessage(msg) {
		return true
	}
	for _, field := range msg.Fields {
		if hasValidateOptions(field) {
			return true
		}
		if field.Message != nil && messageHasValidate(field.Message, seen) {
			return true
		}
	}
	return false
}
