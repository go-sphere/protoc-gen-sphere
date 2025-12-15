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
}

type GenConfig struct {
	omitempty       bool
	omitemptyPrefix string
	swaggerAuth     string
	packageDesc     *template.PackageDesc
}

func NewGenConf(g *protogen.GeneratedFile, conf *Config) *GenConfig {
	pkgDesc := &template.PackageDesc{
		RouterType:  g.QualifiedGoIdent(conf.RouterType),
		ContextType: g.QualifiedGoIdent(conf.ContextType),
		HandlerType: g.QualifiedGoIdent(conf.HandlerType),

		ErrorResponseType: g.QualifiedGoIdent(conf.ErrorRespType),
		DataResponseType:  g.QualifiedGoIdent(conf.DataRespType),

		ServerHandlerWrapperFunc: g.QualifiedGoIdent(conf.ServerHandlerFunc),
	}
	genConf := &GenConfig{
		omitempty:       conf.Omitempty,
		omitemptyPrefix: conf.OmitemptyPrefix,
		swaggerAuth:     conf.SwaggerAuth,
		packageDesc:     pkgDesc,
	}
	return genConf
}
