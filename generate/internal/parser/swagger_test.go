package parser

import (
	"strings"
	"testing"

	validatepb "buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go/buf/validate"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"
)

func TestSwaggerDescription(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "  \n  ", ""},
		{"crlf whitespace only", "  \r\n  ", ""},
		{"single line", " hello ", "hello"},
		{"multi line joined with comma", "line1\nline2\nline3", "line1,line2,line3"},
		{"crlf multi line joined with comma", "line1\r\nline2\r\n", "line1,line2"},
		{"bare cr stripped", "line1\rline2", "line1line2"},
		{"trailing newline trimmed", "line1\nline2\n", "line1,line2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := swaggerDescription(tt.in); got != tt.want {
				t.Errorf("swaggerDescription(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsFieldRequired(t *testing.T) {
	optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING
	i64 := descriptorpb.FieldDescriptorProto_TYPE_INT64

	gtZeroOpts := &descriptorpb.FieldOptions{}
	proto.SetExtension(gtZeroOpts, validatepb.E_Field, &validatepb.FieldRules{
		Type: &validatepb.FieldRules_Int64{Int64: &validatepb.Int64Rules{
			GreaterThan: &validatepb.Int64Rules_Gt{Gt: 0},
		}},
	})

	optReqOpts := &descriptorpb.FieldOptions{}
	proto.SetExtension(optReqOpts, validatepb.E_Field, &validatepb.FieldRules{
		Required: proto.Bool(true),
	})

	optMinLenOpts := &descriptorpb.FieldOptions{}
	proto.SetExtension(optMinLenOpts, validatepb.E_Field, &validatepb.FieldRules{
		Type: &validatepb.FieldRules_String_{String_: &validatepb.StringRules{MinLen: proto.Uint64(1)}},
	})

	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("req.proto"),
		Package: proto.String("req.v1"),
		Syntax:  proto.String("proto3"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("example.com/reqv1;reqv1"),
		},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Req"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: proto.String("uri_gt"), Number: proto.Int32(1), Label: &optional, Type: &i64, Options: gtZeroOpts},
				{Name: proto.String("opt_uri"), Number: proto.Int32(2), Label: &optional, Type: &str, OneofIndex: proto.Int32(0), Proto3Optional: proto.Bool(true)},
				{Name: proto.String("opt_query_req"), Number: proto.Int32(3), Label: &optional, Type: &str, OneofIndex: proto.Int32(1), Proto3Optional: proto.Bool(true), Options: optReqOpts},
				{Name: proto.String("opt_query_min"), Number: proto.Int32(4), Label: &optional, Type: &str, OneofIndex: proto.Int32(2), Proto3Optional: proto.Bool(true), Options: optMinLenOpts},
			},
			OneofDecl: []*descriptorpb.OneofDescriptorProto{
				{Name: proto.String("_opt_uri")},
				{Name: proto.String("_opt_query_req")},
				{Name: proto.String("_opt_query_min")},
			},
		}},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("ReqService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:       proto.String("Get"),
				InputType:  proto.String(".req.v1.Req"),
				OutputType: proto.String(".req.v1.Req"),
			}},
		}},
	}

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"req.proto"},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fd},
	}
	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("protogen.Options.New: %v", err)
	}
	file := plugin.Files[0]
	gen := NewGen(plugin.NewGeneratedFile("req.sphere.pb.go", file.GoImportPath))
	method := file.Services[0].Methods[0]
	fields := make(map[string]*protogen.Field, len(method.Input.Fields))
	for _, f := range method.Input.Fields {
		fields[string(f.Desc.Name())] = f
	}
	mustField := func(name string) *protogen.Field {
		t.Helper()
		f := fields[name]
		if f == nil {
			t.Fatalf("field %s not found", name)
		}
		return f
	}

	uriGt := mustField("uri_gt")
	optURI := mustField("opt_uri")
	optQueryReq := mustField("opt_query_req")
	optQueryMin := mustField("opt_query_min")

	if !isFieldRequired(uriGt, true) {
		t.Error("URI param with int64.gt = 0 and defaultRequired=true should be required")
	}
	if isFieldRequired(uriGt, false) {
		t.Error("query param with int64.gt = 0 and defaultRequired=false should not be required")
	}
	if !isFieldRequired(optQueryReq, false) {
		t.Error("optional query param with buf.validate required=true should be required")
	}
	if isFieldRequired(optQueryMin, false) {
		t.Error("optional query param without required=true should not be required")
	}
	if isFieldRequired(optURI, true) {
		t.Error("isFieldRequired(optional field, true) = true, want false")
	}

	// BuildAnnotations must emit required=true for all path parameters (including
	// optional URI fields and URI fields with non-required validate rules), and
	// required=true for optional query params with (buf.validate.field).required = true.
	block, err := BuildAnnotations(gen, method, &SwagParams{
		Method: "GET",
		Path:   "/v1/{uri_gt}/{opt_uri}",
		PathVars: []ParamsField{
			{Name: "uri_gt", Field: uriGt},
			{Name: "opt_uri", Field: optURI},
		},
		QueryVars: []ParamsField{
			{Name: "opt_query_req", Field: optQueryReq},
			{Name: "opt_query_min", Field: optQueryMin},
		},
		DataResponse:  "httpz.DataResponse",
		ErrorResponse: "httpz.ErrorResponse",
	})
	if err != nil {
		t.Fatalf("BuildAnnotations error = %v", err)
	}
	for _, want := range []string{
		`// @Param uri_gt path integer true "uri_gt"`,
		`// @Param opt_uri path string true "opt_uri"`,
		`// @Param opt_query_req query string true "opt_query_req"`,
		`// @Param opt_query_min query string false "opt_query_min"`,
	} {
		if !strings.Contains(block, want) {
			t.Errorf("BuildAnnotations missing %q:\n%s", want, block)
		}
	}
}

func TestBuildAnnotations_PackageLessAndCRLF(t *testing.T) {
	leading := "line1\r\nline2\r\n"
	fd := &descriptorpb.FileDescriptorProto{
		Name:   proto.String("nopkg.proto"),
		Syntax: proto.String("proto3"),
		Options: &descriptorpb.FileOptions{
			GoPackage: proto.String("example.com/nopkg;nopkg"),
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("Req")},
			{Name: proto.String("Resp")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: proto.String("NoPkgService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:       proto.String("Ping"),
				InputType:  proto.String(".Req"),
				OutputType: proto.String(".Resp"),
			}},
		}},
		SourceCodeInfo: &descriptorpb.SourceCodeInfo{
			Location: []*descriptorpb.SourceCodeInfo_Location{{
				Path:            []int32{6, 0, 2, 0}, // service[0].method[0]
				Span:            []int32{1, 0, 1},
				LeadingComments: &leading,
			}},
		},
	}

	req := &pluginpb.CodeGeneratorRequest{
		FileToGenerate: []string{"nopkg.proto"},
		ProtoFile:      []*descriptorpb.FileDescriptorProto{fd},
	}
	plugin, err := protogen.Options{}.New(req)
	if err != nil {
		t.Fatalf("protogen.Options.New: %v", err)
	}
	file := plugin.Files[0]
	gen := NewGen(plugin.NewGeneratedFile("nopkg.sphere.pb.go", file.GoImportPath))
	method := file.Services[0].Methods[0]

	block, err := BuildAnnotations(gen, method, &SwagParams{
		Method:        "GET",
		Path:          "/ping",
		Auth:          "// @Security ApiKeyAuth\r\n// @Security OAuth2\r\n",
		DataResponse:  "httpz.DataResponse",
		ErrorResponse: "httpz.ErrorResponse",
	})
	if err != nil {
		t.Fatalf("BuildAnnotations error = %v", err)
	}
	for _, want := range []string{
		"// @Description line1,line2",
		"// @Tags NoPkgService",
		"// @Security ApiKeyAuth",
		"// @Security OAuth2",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("BuildAnnotations missing %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "\r") {
		t.Errorf("BuildAnnotations output contains CR:\n%q", block)
	}
}
