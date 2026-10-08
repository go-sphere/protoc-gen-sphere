package http

import (
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// oneofFile builds a POST body:"*" method whose request holds a plain field and
// a proto3 optional field (a synthetic oneof). With realOneof the request also
// carries a real oneof `kind`.
func oneofFile(realOneof bool) *descriptorpb.FileDescriptorProto {
	methodOpts := &descriptorpb.MethodOptions{}
	proto.SetExtension(methodOpts, annotations.E_Http, &annotations.HttpRule{
		Pattern: &annotations.HttpRule_Post{Post: "/v1/search"},
		Body:    "*",
	})
	optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	i64 := descriptorpb.FieldDescriptorProto_TYPE_INT64
	// Real oneofs must be declared before synthetic ones.
	var fields []*descriptorpb.FieldDescriptorProto
	var oneofs []*descriptorpb.OneofDescriptorProto
	if realOneof {
		fields = append(fields,
			&descriptorpb.FieldDescriptorProto{Name: proto.String("text"), JsonName: proto.String("text"), Number: proto.Int32(3), Label: &optional, Type: &str, OneofIndex: proto.Int32(0)},
			&descriptorpb.FieldDescriptorProto{Name: proto.String("number"), JsonName: proto.String("number"), Number: proto.Int32(4), Label: &optional, Type: &i64, OneofIndex: proto.Int32(0)},
		)
		oneofs = append(oneofs, &descriptorpb.OneofDescriptorProto{Name: proto.String("kind")})
	}
	fields = append(fields,
		&descriptorpb.FieldDescriptorProto{Name: proto.String("id"), JsonName: proto.String("id"), Number: proto.Int32(1), Label: &optional, Type: &str},
		&descriptorpb.FieldDescriptorProto{Name: proto.String("note"), JsonName: proto.String("note"), Number: proto.Int32(2), Label: &optional, Type: &str, OneofIndex: proto.Int32(int32(len(oneofs))), Proto3Optional: proto.Bool(true)},
	)
	oneofs = append(oneofs, &descriptorpb.OneofDescriptorProto{Name: proto.String("_note")})
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("oneof.proto"),
		Package: proto.String("oneof.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{GoPackage: proto.String("github.com/example/oneof;oneof")},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("SearchRequest"), Field: fields, OneofDecl: oneofs},
			{Name: proto.String("SearchResponse")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("OneofService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:       proto.String("Search"),
				InputType:  proto.String(".oneof.v1.SearchRequest"),
				OutputType: proto.String(".oneof.v1.SearchResponse"),
				Options:    methodOpts,
			}},
		}},
	}
}

// TestRequestOneofRejected pins that a real oneof in a request fails generation
// regardless of fail_on_warn: no generated binder can fill it, so emitting a
// handler would drop the client's value silently. Proto3 optional fields are
// synthetic oneofs and still generate.
func TestRequestOneofRejected(t *testing.T) {
	want := "method `OneofService.Search` request message contains oneof `kind` (message `oneof.v1.SearchRequest`), which generated HTTP handlers cannot bind: the JSON body is decoded with encoding/json, which ignores oneof fields, and query/uri/header/form binding skips them; replace the oneof with separate optional fields. File: `oneof.proto`, Message: `SearchRequest`"
	plugin := newPlugin(t, oneofFile(true))
	_, err := GenerateFile(plugin, plugin.Files[0], DefaultConfig())
	if err == nil || err.Error() != want {
		t.Fatalf("GenerateFile() error = %v, want %q", err, want)
	}

	plugin = newPlugin(t, oneofFile(false))
	genFile, err := GenerateFile(plugin, plugin.Files[0], DefaultConfig())
	if err != nil {
		t.Fatalf("proto3 optional only: GenerateFile() error = %v", err)
	}
	content, err := genFile.Content()
	if err != nil {
		t.Fatalf("Content() error = %v", err)
	}
	mustParseGo(t, "oneof.sphere.pb.go", content)
	if !strings.Contains(string(content), "ctx.BindJSON(&in)") {
		t.Errorf("request must still bind the JSON body:\n%s", content)
	}
}
