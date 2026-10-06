package http

import (
	"strings"
	"testing"

	bindingpb "github.com/go-sphere/binding/sphere/binding"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// oneofFile builds a GET method whose request carries a top-level oneof.
// oneofLoc is the oneof's default_oneof_location and memberLoc the location
// of its first member; nil leaves the option unset. The request also holds a
// plain QUERY field and a proto3 optional QUERY field (a synthetic oneof),
// neither of which may be reported.
func oneofFile(oneofLoc, memberLoc *bindingpb.BindingLocation) *descriptorpb.FileDescriptorProto {
	methodOpts := &descriptorpb.MethodOptions{}
	proto.SetExtension(methodOpts, annotations.E_Http, &annotations.HttpRule{
		Pattern: &annotations.HttpRule_Get{Get: "/v1/search"},
	})
	queryOpts := func() *descriptorpb.FieldOptions {
		opts := &descriptorpb.FieldOptions{}
		proto.SetExtension(opts, bindingpb.E_Location, bindingpb.BindingLocation_BINDING_LOCATION_QUERY)
		return opts
	}
	oneofOpts := &descriptorpb.OneofOptions{}
	if oneofLoc != nil {
		proto.SetExtension(oneofOpts, bindingpb.E_DefaultOneofLocation, *oneofLoc)
	}
	var memberOpts *descriptorpb.FieldOptions
	if memberLoc != nil {
		memberOpts = &descriptorpb.FieldOptions{}
		proto.SetExtension(memberOpts, bindingpb.E_Location, *memberLoc)
	}
	optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	i64 := descriptorpb.FieldDescriptorProto_TYPE_INT64
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("oneof.proto"),
		Package: proto.String("oneof.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{GoPackage: proto.String("github.com/example/oneof;oneof")},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("SearchRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("id"), JsonName: proto.String("id"), Number: proto.Int32(1), Label: &optional, Type: &str, Options: queryOpts()},
					{Name: proto.String("text"), JsonName: proto.String("text"), Number: proto.Int32(2), Label: &optional, Type: &str, OneofIndex: proto.Int32(0), Options: memberOpts},
					{Name: proto.String("number"), JsonName: proto.String("number"), Number: proto.Int32(3), Label: &optional, Type: &i64, OneofIndex: proto.Int32(0)},
					{Name: proto.String("note"), JsonName: proto.String("note"), Number: proto.Int32(4), Label: &optional, Type: &str, OneofIndex: proto.Int32(1), Proto3Optional: proto.Bool(true), Options: queryOpts()},
				},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{
					{Name: proto.String("kind"), Options: oneofOpts},
					{Name: proto.String("_note")},
				},
			},
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

// TestOneofBindingWarnings pins the "oneof fields bind only via the JSON body"
// contract: a non-JSON location declared on a real oneof or its member is
// reported as a generation warning (an error under fail_on_warn) instead of
// being silently ignored, while existing protos keep generating.
func TestOneofBindingWarnings(t *testing.T) {
	query := bindingpb.BindingLocation_BINDING_LOCATION_QUERY
	header := bindingpb.BindingLocation_BINDING_LOCATION_HEADER
	jsonLoc := bindingpb.BindingLocation_BINDING_LOCATION_JSON
	oneofWarning := "method `OneofService.Search` oneof `kind` declares BINDING_LOCATION_QUERY, but oneof fields bind only via the JSON body; the declaration is ignored. File: `oneof.proto`, Message: `SearchRequest`"
	fieldWarning := "method `OneofService.Search` field `text` declares BINDING_LOCATION_HEADER, but oneof fields bind only via the JSON body; the declaration is ignored. File: `oneof.proto`, Message: `SearchRequest`"

	tests := []struct {
		name      string
		oneofLoc  *bindingpb.BindingLocation
		memberLoc *bindingpb.BindingLocation
		want      []string
	}{
		{name: "no declarations", want: nil},
		{name: "JSON declarations are consistent", oneofLoc: &jsonLoc, memberLoc: &jsonLoc, want: nil},
		{name: "default_oneof_location", oneofLoc: &query, want: []string{oneofWarning}},
		{name: "member location", memberLoc: &header, want: []string{fieldWarning}},
		{name: "both, in declaration order", oneofLoc: &query, memberLoc: &header, want: []string{oneofWarning, fieldWarning}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fd := oneofFile(tt.oneofLoc, tt.memberLoc)

			var content []byte
			stderr := captureStderr(t, func() {
				plugin := newPlugin(t, fd)
				genFile, err := GenerateFile(plugin, plugin.Files[0], DefaultConfig())
				if err != nil {
					t.Fatalf("GenerateFile() error = %v", err)
				}
				if content, err = genFile.Content(); err != nil {
					t.Fatalf("Content() error = %v", err)
				}
			})
			if got := strings.Count(stderr, "WARN"); got != len(tt.want) {
				t.Errorf("warning count = %d, want %d\n%s", got, len(tt.want), stderr)
			}
			last := -1
			for _, want := range tt.want {
				i := strings.Index(stderr, want)
				if i < 0 {
					t.Errorf("stderr does not contain %q\n%s", want, stderr)
					continue
				}
				if i < last {
					t.Errorf("warning %q is out of declaration order\n%s", want, stderr)
				}
				last = i
			}
			// The declaration is not honored: only the plain and the proto3
			// optional QUERY fields are bound, never a header.
			mustParseGo(t, "oneof.sphere.pb.go", content)
			if strings.Contains(string(content), "BindHeader") {
				t.Errorf("oneof member must not be bound to a header:\n%s", content)
			}
			if !strings.Contains(string(content), "ctx.BindQuery(&in)") {
				t.Errorf("QUERY fields must still be bound:\n%s", content)
			}

			cfg := DefaultConfig()
			cfg.FailOnWarn = true
			plugin := newPlugin(t, fd)
			_, err := GenerateFile(plugin, plugin.Files[0], cfg)
			switch {
			case len(tt.want) == 0 && err != nil:
				t.Errorf("fail_on_warn GenerateFile() error = %v, want nil", err)
			case len(tt.want) > 0 && (err == nil || err.Error() != tt.want[0]):
				t.Errorf("fail_on_warn GenerateFile() error = %v, want %q", err, tt.want[0])
			}
		})
	}
}
