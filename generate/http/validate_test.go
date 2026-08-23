package http

import (
	"testing"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/testutil"
	"google.golang.org/protobuf/compiler/protogen"
)

func TestRequestNeedsValidateNested(t *testing.T) {
	set := testutil.LoadDescriptorSet(t, "testdata/pb/validate.pb")
	plugin := testutil.MustCreatePlugin(t, set, "validate.proto")
	file := testutil.FileToGenerate(t, plugin)

	byName := map[string]*protogen.Message{}
	for _, m := range file.Messages {
		byName[m.GoIdent.GoName] = m
	}

	if msg := byName["CreateRequest"]; msg == nil {
		t.Fatal("CreateRequest not found")
	} else if !requestNeedsValidate(msg) {
		t.Error("top-level field rules should need validate")
	}

	if msg := byName["NestedCreateRequest"]; msg == nil {
		t.Fatal("NestedCreateRequest not found")
	} else if !requestNeedsValidate(msg) {
		t.Error("nested field rules should need validate")
	}

	if msg := byName["CreateResponse"]; msg == nil {
		t.Fatal("CreateResponse not found")
	} else if requestNeedsValidate(msg) {
		t.Error("message without rules should not need validate")
	}
}
