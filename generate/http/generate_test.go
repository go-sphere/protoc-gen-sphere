package http

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	validatepb "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	bindingpb "github.com/go-sphere/binding/sphere/binding"
	"google.golang.org/genproto/googleapis/api/annotations"
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

// TestGenerateFile_StreamingOnly verifies that services with only
// client-streaming methods are skipped even when omit-empty is disabled.
// (Server-streaming methods do generate code — see the streaming golden.)
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
	cfg.OmitEmpty = false
	genFile, err := GenerateFile(plugin, plugin.Files[0], cfg)
	if err != nil {
		t.Fatalf("GenerateFile failed: %v", err)
	}
	if genFile != nil {
		t.Error("expected nil for streaming-only service, got non-nil")
	}
}

func TestNewGeneratorSnapshotsConfig(t *testing.T) {
	cfg := DefaultConfig()
	generator, err := NewGenerator(cfg)
	if err != nil {
		t.Fatalf("NewGenerator() error = %v", err)
	}
	cfg.OmitEmpty = false
	if !generator.cfg.OmitEmpty {
		t.Fatal("Generator configuration changed after caller mutation")
	}
	if _, err := NewGenerator(nil); err == nil {
		t.Fatal("NewGenerator(nil) error = nil")
	}
}

func TestGenerateFile_GoPackageContext(t *testing.T) {
	unaryOpts := &descriptorpb.MethodOptions{}
	proto.SetExtension(unaryOpts, annotations.E_Http, &annotations.HttpRule{
		Pattern: &annotations.HttpRule_Get{Get: "/v1/ping"},
	})
	streamOpts := &descriptorpb.MethodOptions{}
	proto.SetExtension(streamOpts, annotations.E_Http, &annotations.HttpRule{
		Pattern: &annotations.HttpRule_Get{Get: "/v1/watch"},
	})

	ctxFD := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("ctx.proto"),
		Package: proto.String("ctx.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("example.com/context;context"),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("PingReq")},
			{Name: proto.String("PingResp")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("CtxService"),
			Method: []*descriptorpb.MethodDescriptorProto{
				{
					Name:       proto.String("Ping"),
					InputType:  proto.String(".ctx.v1.PingReq"),
					OutputType: proto.String(".ctx.v1.PingResp"),
					Options:    unaryOpts,
				},
				{
					Name:            proto.String("Watch"),
					InputType:       proto.String(".ctx.v1.PingReq"),
					OutputType:      proto.String(".ctx.v1.PingResp"),
					ServerStreaming: proto.Bool(true),
					Options:         streamOpts,
				},
			},
		}},
	}

	t.Run("self package named context", func(t *testing.T) {
		plugin := newPlugin(t, ctxFD)
		genFile, err := GenerateFile(plugin, plugin.Files[0], DefaultConfig())
		if err != nil {
			t.Fatalf("GenerateFile() error = %v", err)
		}
		content, err := genFile.Content()
		if err != nil {
			t.Fatalf("Content() error = %v", err)
		}
		src := string(content)
		parsed, err := parser.ParseFile(token.NewFileSet(), "ctx.sphere.pb.go", content, parser.ParseComments)
		if err != nil {
			t.Fatalf("generated file is not valid Go: %v\n%s", err, src)
		}
		var ctxIdent string
		for _, imp := range parsed.Imports {
			if imp.Path.Value == `"context"` && imp.Name != nil {
				ctxIdent = imp.Name.Name
			}
		}
		if ctxIdent == "" {
			t.Fatalf("expected explicit context import\n%s", src)
		}
		for _, want := range []string{
			"package context",
			"var _ = new(" + ctxIdent + ".Context)",
			"Ping(" + ctxIdent + ".Context, *PingReq) (*PingResp, error)",
			"Watch(" + ctxIdent + ".Context, *PingReq, func(*PingResp) error) error",
		} {
			if !strings.Contains(src, want) {
				t.Errorf("generated file missing %q\n%s", want, src)
			}
		}
	})

	t.Run("imported package named context disambiguated", func(t *testing.T) {
		svcFD := &descriptorpb.FileDescriptorProto{
			Name:       proto.String("svc.proto"),
			Package:    proto.String("svc.v1"),
			Syntax:     proto.String("proto3"),
			Dependency: []string{"ctx.proto"},
			Options: &descriptorpb.FileOptions{
				GoPackage: proto.String("example.com/svc;svc"),
			},
			Service: []*descriptorpb.ServiceDescriptorProto{{
				Name: proto.String("SvcService"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name:       proto.String("Ping"),
					InputType:  proto.String(".ctx.v1.PingReq"),
					OutputType: proto.String(".ctx.v1.PingResp"),
					Options:    unaryOpts,
				}},
			}},
		}
		req := &pluginpb.CodeGeneratorRequest{
			FileToGenerate: []string{"svc.proto"},
			ProtoFile:      []*descriptorpb.FileDescriptorProto{ctxFD, svcFD},
		}
		plugin, err := protogen.Options{}.New(req)
		if err != nil {
			t.Fatalf("protogen.Options.New: %v", err)
		}
		var svcFile *protogen.File
		for _, f := range plugin.Files {
			if f.Generate {
				svcFile = f
			}
		}
		if svcFile == nil {
			t.Fatal("target file svc.proto not found")
		}
		genFile, err := GenerateFile(plugin, svcFile, DefaultConfig())
		if err != nil {
			t.Fatalf("GenerateFile() error = %v", err)
		}
		content, err := genFile.Content()
		if err != nil {
			t.Fatalf("Content() error = %v", err)
		}
		src := string(content)
		parsed, err := parser.ParseFile(token.NewFileSet(), "svc.sphere.pb.go", content, parser.ParseComments)
		if err != nil {
			t.Fatalf("generated file is not valid Go: %v\n%s", err, src)
		}
		var stdAlias, depAlias string
		for _, imp := range parsed.Imports {
			switch imp.Path.Value {
			case `"context"`:
				if imp.Name != nil {
					stdAlias = imp.Name.Name
				}
			case `"example.com/context"`:
				if imp.Name != nil {
					depAlias = imp.Name.Name
				}
			}
		}
		if stdAlias == "" || depAlias == "" || stdAlias == depAlias {
			t.Fatalf("expected distinct context aliases, got std=%q dep=%q\n%s", stdAlias, depAlias, src)
		}
		wantSig := "Ping(" + stdAlias + ".Context, *" + depAlias + ".PingReq) (*" + depAlias + ".PingResp, error)"
		if !strings.Contains(src, wantSig) {
			t.Errorf("generated file missing %q\n%s", wantSig, src)
		}
	})
}

func TestGenerateFile_CustomStreamAndSkippedValidatedRPC(t *testing.T) {
	optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	reqOpts := &descriptorpb.FieldOptions{}
	proto.SetExtension(reqOpts, validatepb.E_Field, &validatepb.FieldRules{
		Required: proto.Bool(true),
	})

	getOpts := &descriptorpb.MethodOptions{}
	proto.SetExtension(getOpts, annotations.E_Http, &annotations.HttpRule{
		Pattern: &annotations.HttpRule_Get{Get: "/v1/ping"},
	})
	streamOpts := &descriptorpb.MethodOptions{}
	proto.SetExtension(streamOpts, annotations.E_Http, &annotations.HttpRule{
		Pattern: &annotations.HttpRule_Get{Get: "/v1/stream"},
	})

	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("custom.proto"),
		Package: proto.String("custom.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("example.com/custom;custom"),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("ValidatedSkippedReq"),
				Field: []*descriptorpb.FieldDescriptorProto{{
					Name:    proto.String("name"),
					Number:  proto.Int32(1),
					Label:   &optional,
					Type:    &str,
					Options: reqOpts,
				}},
			},
			{Name: proto.String("PlainReq")},
			{Name: proto.String("PlainResp")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("CustomService"),
			Method: []*descriptorpb.MethodDescriptorProto{
				// No google.api.http rule: omitted under OmitEmpty=true even though its
				// input carries a validation rule.
				{
					Name:       proto.String("SkippedValidated"),
					InputType:  proto.String(".custom.v1.ValidatedSkippedReq"),
					OutputType: proto.String(".custom.v1.PlainResp"),
				},
				{
					Name:       proto.String("Ping"),
					InputType:  proto.String(".custom.v1.PlainReq"),
					OutputType: proto.String(".custom.v1.PlainResp"),
					Options:    getOpts,
				},
				{
					Name:            proto.String("Stream"),
					InputType:       proto.String(".custom.v1.PlainReq"),
					OutputType:      proto.String(".custom.v1.PlainResp"),
					ServerStreaming: proto.Bool(true),
					Options:         streamOpts,
				},
			},
		}},
	}

	cfg := DefaultConfig()
	cfg.RouterType = protogen.GoImportPath("example.com/myhttp").Ident("Router")
	cfg.ContextType = protogen.GoImportPath("example.com/myhttp").Ident("Context")
	cfg.HandlerType = protogen.GoImportPath("example.com/myhttp").Ident("Handler")
	cfg.ErrorRespType = protogen.GoImportPath("example.com/myresp").Ident("ErrorResponse")
	cfg.DataRespType = protogen.GoImportPath("example.com/myresp").Ident("DataResponse")
	cfg.ServerHandlerFunc = protogen.GoImportPath("example.com/myresp").Ident("WithJSON")
	cfg.StreamHandlerFunc = protogen.GoImportPath("example.com/mysse").Ident("WithCustomSSE")
	cfg.StreamType = protogen.GoImportPath("example.com/mysse").Ident("CustomStream")

	plugin := newPlugin(t, fd)
	genFile, err := GenerateFile(plugin, plugin.Files[0], cfg)
	if err != nil {
		t.Fatalf("GenerateFile() error = %v", err)
	}
	content, err := genFile.Content()
	if err != nil {
		t.Fatalf("Content() error = %v", err)
	}
	src := string(content)
	parsed, err := parser.ParseFile(token.NewFileSet(), "custom.sphere.pb.go", content, parser.ParseComments)
	if err != nil {
		t.Fatalf("generated file is not valid Go: %v\n%s", err, src)
	}

	// Verify custom StreamHandlerFunc/StreamType are used directly without
	// uninstantiated `var _ = new(...)` keep-alive dummies, and that neither
	// httpx nor protovalidate is imported for the skipped validated RPC.
	if strings.Contains(src, "var _ = new(mysse.") {
		t.Errorf("generic stream types must not emit uninstantiated new(...) keep-alive:\n%s", src)
	}
	for _, want := range []string{
		"mysse.WithCustomSSE(func(ctx myhttp.Context) (mysse.CustomStream[*PlainResp], error)",
		"myresp.WithJSON(func(ctx myhttp.Context) (*PlainResp, error)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated file missing %q\n%s", want, src)
		}
	}
	for _, imp := range parsed.Imports {
		switch imp.Path.Value {
		case `"github.com/go-sphere/httpx"`, `"buf.build/go/protovalidate"`:
			t.Errorf("unexpected import %s when only skipped RPC needs validation\n%s", imp.Path.Value, src)
		}
	}
}

func TestGenerateFile_CustomNonHTTPXRouterWithActiveValidatedRPC(t *testing.T) {
	optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	reqOpts := &descriptorpb.FieldOptions{}
	proto.SetExtension(reqOpts, validatepb.E_Field, &validatepb.FieldRules{
		Required: proto.Bool(true),
	})
	postOpts := &descriptorpb.MethodOptions{}
	proto.SetExtension(postOpts, annotations.E_Http, &annotations.HttpRule{
		Pattern: &annotations.HttpRule_Post{Post: "/v1/items"},
		Body:    "*",
	})

	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("active_val.proto"),
		Package: proto.String("activeval.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("example.com/activeval;activeval"),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("CreateReq"),
				Field: []*descriptorpb.FieldDescriptorProto{{
					Name:    proto.String("name"),
					Number:  proto.Int32(1),
					Label:   &optional,
					Type:    &str,
					Options: reqOpts,
				}},
			},
			{Name: proto.String("CreateResp")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("ItemService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:       proto.String("Create"),
				InputType:  proto.String(".activeval.v1.CreateReq"),
				OutputType: proto.String(".activeval.v1.CreateResp"),
				Options:    postOpts,
			}},
		}},
	}

	cfg := DefaultConfig()
	cfg.RouterType = protogen.GoImportPath("example.com/myhttp").Ident("Router")
	cfg.ContextType = protogen.GoImportPath("example.com/myhttp").Ident("Context")
	cfg.HandlerType = protogen.GoImportPath("example.com/myhttp").Ident("Handler")

	plugin := newPlugin(t, fd)
	genFile, err := GenerateFile(plugin, plugin.Files[0], cfg)
	if err != nil {
		t.Fatalf("GenerateFile() error = %v", err)
	}
	content, err := genFile.Content()
	if err != nil {
		t.Fatalf("Content() error = %v", err)
	}
	src := string(content)
	mustParseGo(t, "active_val.sphere.pb.go", content)
	if strings.Contains(src, "var _ = new(httpx.BadRequestError)") {
		t.Errorf("validateErrorFunc must not emit var _ = new(httpx.BadRequestError):\n%s", src)
	}
	if !strings.Contains(src, "httpx.BadRequestError(err)") {
		t.Errorf("expected httpx.BadRequestError(err) call:\n%s", src)
	}
}

func TestGenerateFile_AdditionalBindingsSubsetURIAndResponseBodyNilGuard(t *testing.T) {
	optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	strType := descriptorpb.FieldDescriptorProto_TYPE_STRING

	uriFieldOpts := &descriptorpb.FieldOptions{}
	proto.SetExtension(uriFieldOpts, bindingpb.E_Location, bindingpb.BindingLocation_BINDING_LOCATION_URI)

	getOpts := &descriptorpb.MethodOptions{}
	proto.SetExtension(getOpts, annotations.E_Http, &annotations.HttpRule{
		Pattern:      &annotations.HttpRule_Get{Get: "/v1/orgs/{org_id}/users/{user_id}"},
		ResponseBody: "payload",
		AdditionalBindings: []*annotations.HttpRule{
			{
				Pattern:      &annotations.HttpRule_Get{Get: "/v1/users/{user_id}"},
				ResponseBody: "payload",
			},
		},
	})

	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("subset_uri.proto"),
		Package: proto.String("subset.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("example.com/subset;subset"),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("GetUserReq"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("org_id"), Number: proto.Int32(1), Label: &optional, Type: &strType, Options: uriFieldOpts},
					{Name: proto.String("user_id"), Number: proto.Int32(2), Label: &optional, Type: &strType, Options: uriFieldOpts},
				},
			},
			{
				Name: proto.String("GetUserResp"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("payload"), Number: proto.Int32(1), Label: &optional, Type: &strType},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("UserService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:       proto.String("GetUser"),
				InputType:  proto.String(".subset.v1.GetUserReq"),
				OutputType: proto.String(".subset.v1.GetUserResp"),
				Options:    getOpts,
			}},
		}},
	}

	plugin := newPlugin(t, fd)
	genFile, err := GenerateFile(plugin, plugin.Files[0], DefaultConfig())
	if err != nil {
		t.Fatalf("GenerateFile() error = %v", err)
	}
	content, err := genFile.Content()
	if err != nil {
		t.Fatalf("Content() error = %v", err)
	}
	src := string(content)
	mustParseGo(t, "subset_uri.sphere.pb.go", content)
	for _, want := range []string{
		`r.Handle("GET", "/v1/orgs/:org_id/users/:user_id"`,
		`r.Handle("GET", "/v1/users/:user_id"`,
		"if out == nil {\n\t\t\treturn \"\", nil\n\t\t}",
		"return out.Payload, nil",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated file missing %q\n%s", want, src)
		}
	}
}
