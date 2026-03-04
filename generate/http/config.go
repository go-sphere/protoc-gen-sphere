package http

import (
	"github.com/go-sphere/protoc-gen-sphere/generate/template"
	"google.golang.org/protobuf/compiler/protogen"
)

type Config struct {
	Omitempty       bool
	OmitemptyPrefix string
	SwaggerAuth     string
	TemplateFile    string

	RouterType    protogen.GoIdent
	ContextType   protogen.GoIdent
	HandlerType   protogen.GoIdent
	ErrorRespType protogen.GoIdent
	DataRespType  protogen.GoIdent

	ServerHandlerFunc protogen.GoIdent
	ContextLoadFunc   string
}

type GenConfig struct {
	omitempty       bool
	omitemptyPrefix string
	swaggerAuth     string
	packageDesc     *template.PackageDesc
}
