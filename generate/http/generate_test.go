package http

import (
	"testing"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

// newPlugin builds a plugin from a single hand-written file descriptor (no
// imports), so plugin.Files[0] is safely the target.
func newPlugin(t *testing.T, fd *descriptorpb.FileDescriptorProto) *protogen.Plugin {
	t.Helper()
	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{fd.GetName()},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fd},
	}
	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("failed to create plugin: %v", err)
	}
	return plugin
}

// TestGenerateFile_EmptyService verifies that a file whose service has no
// (non-streaming, http-annotated) methods produces no output.
func TestGenerateFile_EmptyService(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("empty.proto"),
		Package: proto.String("api.v1"),
		Options: &descriptorpb.FileOptions{GoPackage: proto.String("github.com/example/api;api")},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{Name: proto.String("EmptyService")},
		},
	}
	plugin := newPlugin(t, fd)

	genFile, err := GenerateFile(plugin, plugin.Files[0], DefaultConfig())
	if err != nil {
		t.Fatalf("GenerateFile failed: %v", err)
	}
	if genFile != nil {
		t.Error("expected nil for empty service, got non-nil")
	}
}

// TestGenerateFile_NoService verifies that a file with no services produces no output.
func TestGenerateFile_NoService(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("noservice.proto"),
		Package: proto.String("api.v1"),
		Options: &descriptorpb.FileOptions{GoPackage: proto.String("github.com/example/api;api")},
	}
	plugin := newPlugin(t, fd)

	genFile, err := GenerateFile(plugin, plugin.Files[0], DefaultConfig())
	if err != nil {
		t.Fatalf("GenerateFile failed: %v", err)
	}
	if genFile != nil {
		t.Error("expected nil for file with no services, got non-nil")
	}
}

// TestGenerateFile_StreamingOnly verifies that streaming-only services are
// skipped even when omitempty is disabled.
func TestGenerateFile_StreamingOnly(t *testing.T) {
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("streaming.proto"),
		Package: proto.String("api.v1"),
		Options: &descriptorpb.FileOptions{GoPackage: proto.String("github.com/example/api;api")},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("StreamRequest")},
			{Name: proto.String("StreamResponse")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("StreamService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:            proto.String("StreamData"),
						InputType:       proto.String(".api.v1.StreamRequest"),
						OutputType:      proto.String(".api.v1.StreamResponse"),
						ClientStreaming: proto.Bool(true),
					},
				},
			},
		},
	}
	plugin := newPlugin(t, fd)

	cfg := DefaultConfig()
	cfg.Omitempty = false
	genFile, err := GenerateFile(plugin, plugin.Files[0], cfg)
	if err != nil {
		t.Fatalf("GenerateFile failed: %v", err)
	}
	if genFile != nil {
		t.Error("expected nil for streaming-only service, got non-nil")
	}
}
