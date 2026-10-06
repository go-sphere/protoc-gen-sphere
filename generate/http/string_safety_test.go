package http

import (
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// specialCharFile builds a single-file descriptor whose only method is bound
// by rule, so tests can feed paths and methods that no checked-in fixture
// would carry.
func specialCharFile(rule *annotations.HttpRule) *descriptorpb.FileDescriptorProto {
	opts := &descriptorpb.MethodOptions{}
	proto.SetExtension(opts, annotations.E_Http, rule)
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("special.proto"),
		Package: proto.String("special.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{GoPackage: proto.String("github.com/example/special;special")},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("Req")},
			{Name: proto.String("Resp")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("SpecialService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:       proto.String("Do"),
				InputType:  proto.String(".special.v1.Req"),
				OutputType: proto.String(".special.v1.Resp"),
				Options:    opts,
			}},
		}},
	}
}

// TestStringLiteralsAreQuoted feeds a path carrying a double quote: it must
// reach the Go output as an escaped string literal and the file must still
// parse. (A quote in a custom HTTP method never gets this far: swagspec only
// accepts plain verbs for @Router.)
func TestStringLiteralsAreQuoted(t *testing.T) {
	plugin := newPlugin(t, specialCharFile(&annotations.HttpRule{
		Pattern: &annotations.HttpRule_Post{Post: `/v1/say"hi`},
		Body:    "*",
	}))
	genFile, err := GenerateFile(plugin, plugin.Files[0], DefaultConfig())
	if err != nil {
		t.Fatalf("GenerateFile() error = %v", err)
	}
	content, err := genFile.Content()
	if err != nil {
		t.Fatalf("Content() error = %v", err)
	}
	mustParseGo(t, "special.sphere.pb.go", content)
	for _, want := range []string{
		`{OperationSpecialServiceDo, "POST", "/v1/say\"hi"}`,
		`r.Handle("POST", "/v1/say\"hi", _SpecialService_Do0_HTTP_Handler(srv))`,
		`const OperationSpecialServiceDo = "/special.v1.SpecialService/Do"`,
	} {
		if !strings.Contains(string(content), want) {
			t.Errorf("generated file does not contain %q\n%s", want, content)
		}
	}
}

// TestNewlinesRejected guards the Swagger comment block: goString makes the
// route and method safe inside Go string literals, but they are also written
// verbatim into the @Router comment line, where a newline would end the
// comment and inject source. swagspec.Operation.Validate rejects both, so
// generation must fail instead of emitting a corrupted file.
func TestNewlinesRejected(t *testing.T) {
	tests := []struct {
		name string
		rule *annotations.HttpRule
		want string
	}{
		{
			name: "newline in path",
			rule: &annotations.HttpRule{
				Pattern: &annotations.HttpRule_Post{Post: "/v1/say\"hi\nfunc init() {}"},
				Body:    "*",
			},
			want: "method `SpecialService.Do`: @Router path must be a single line",
		},
		{
			name: "newline in custom method",
			rule: &annotations.HttpRule{
				Pattern: &annotations.HttpRule_Custom{Custom: &annotations.CustomHttpPattern{
					Kind: "LINK\n",
					Path: "/v1/link",
				}},
				Body: "*",
			},
			want: "method `SpecialService.Do`: @Router method \"link\\n\" is not a valid verb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := newPlugin(t, specialCharFile(tt.rule))
			_, err := GenerateFile(plugin, plugin.Files[0], DefaultConfig())
			if err == nil {
				t.Fatal("GenerateFile() error = nil, want a newline rejection")
			}
			if got := err.Error(); got != tt.want {
				t.Errorf("error = %q, want %q", got, tt.want)
			}
		})
	}
}
