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
