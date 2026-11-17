package parser

import (
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestProtoTypeToGoType_OptionalAndPresence(t *testing.T) {
	optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	repeated := descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	bytesT := descriptorpb.FieldDescriptorProto_TYPE_BYTES
	enumT := descriptorpb.FieldDescriptorProto_TYPE_ENUM
	msgT := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE

	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("types.proto"),
		Package: proto.String("types.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("example.com/typesv1;typesv1"),
		},
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: proto.String("Kind"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: proto.String("KIND_UNSPECIFIED"), Number: proto.Int32(0)},
			},
		}},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("Child")},
			{
				Name: proto.String("Payload"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("plain_str"), Number: proto.Int32(1), Label: &optional, Type: &str},
					{Name: proto.String("opt_str"), Number: proto.Int32(2), Label: &optional, Type: &str, OneofIndex: proto.Int32(1), Proto3Optional: proto.Bool(true)},
					{Name: proto.String("opt_bytes"), Number: proto.Int32(3), Label: &optional, Type: &bytesT, OneofIndex: proto.Int32(2), Proto3Optional: proto.Bool(true)},
					{Name: proto.String("opt_enum"), Number: proto.Int32(4), Label: &optional, Type: &enumT, TypeName: proto.String(".types.v1.Kind"), OneofIndex: proto.Int32(3), Proto3Optional: proto.Bool(true)},
					{Name: proto.String("opt_msg"), Number: proto.Int32(5), Label: &optional, Type: &msgT, TypeName: proto.String(".types.v1.Child"), OneofIndex: proto.Int32(4), Proto3Optional: proto.Bool(true)},
					{Name: proto.String("oneof_str"), Number: proto.Int32(6), Label: &optional, Type: &str, OneofIndex: proto.Int32(0)},
					{Name: proto.String("list_msg"), Number: proto.Int32(7), Label: &repeated, Type: &msgT, TypeName: proto.String(".types.v1.Child")},
				},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{
					{Name: proto.String("choice")},
					{Name: proto.String("_opt_str")},
					{Name: proto.String("_opt_bytes")},
					{Name: proto.String("_opt_enum")},
					{Name: proto.String("_opt_msg")},
				},
			},
		},
	}

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"types.proto"},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fd},
	}
	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("protogen.Options.New: %v", err)
	}
	file := plugin.Files[0]
	gen := NewGen(plugin.NewGeneratedFile("types.sphere.pb.go", file.GoImportPath))

	var payload *protogen.Message
	for _, m := range file.Messages {
		if m.Desc.Name() == "Payload" {
			payload = m
		}
	}
	if payload == nil {
		t.Fatal("Payload message not found")
	}

	fields := make(map[string]*protogen.Field, len(payload.Fields))
	for _, f := range payload.Fields {
		fields[string(f.Desc.Name())] = f
	}

	tests := []struct {
		field      string
		usePtrMsg  bool
		wantGoType string
	}{
		{"plain_str", true, "string"},
		{"opt_str", true, "*string"},
		{"opt_bytes", true, "[]byte"},
		{"opt_enum", true, "*Kind"},
		{"opt_msg", true, "*Child"},
		{"opt_msg", false, "Child"},
		{"oneof_str", true, "string"},
		{"list_msg", false, "[]*Child"},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			f := fields[tt.field]
			if f == nil {
				t.Fatalf("field %s not found", tt.field)
			}
			if got := ProtoTypeToGoType(gen, f, tt.usePtrMsg); got != tt.wantGoType {
				t.Errorf("ProtoTypeToGoType(%s, %v) = %q, want %q", tt.field, tt.usePtrMsg, got, tt.wantGoType)
			}
		})
	}
}
