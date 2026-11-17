package http

import (
	"fmt"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/parser"
	"github.com/go-sphere/protoc-gen-sphere/generate/internal/template"
	"google.golang.org/protobuf/compiler/protogen"
)

const validatePackage = protogen.GoImportPath("buf.build/go/protovalidate")

// validateErrorFunc wraps a validation failure so the response is 400 rather
// than the 500 an unclassified error maps to.
var validateErrorFunc = protogen.GoImportPath(defaultHTTPxPackage).Ident("BadRequestError")

// collectGoImport emits the `var _ = ...` lines that keep referenced-but-unused
// imports alive in the generated file, and, when any generated method needs
// validation, wires up the validate func on the package descriptor.
func collectGoImport(file *protogen.File, g *parser.GeneratedFile, cfg *Config, fileCfg *fileConfig, services []*template.ServiceDesc) []string {
	lines := make([]string, 0)
	didImport := make(map[protogen.GoImportPath]bool)
	didImport[file.GoImportPath] = true
	for _, ident := range g.Dummies() {
		if !didImport[ident.GoImportPath] {
			didImport[ident.GoImportPath] = true
			lines = append(lines, fmt.Sprintf("var _ = new(%s)", g.QualifiedGoIdent(ident)))
		}
	}
	if !didImport[cfg.ServerHandlerFunc.GoImportPath] {
		didImport[cfg.ServerHandlerFunc.GoImportPath] = true
		lines = append(lines, fmt.Sprintf("var _ = %s[any]", g.QualifiedUsedGoIdent(cfg.ServerHandlerFunc)))
	}
	if !didImport[cfg.DataRespType.GoImportPath] {
		didImport[cfg.DataRespType.GoImportPath] = true
		lines = append(lines, fmt.Sprintf("var _ = new(%s[any])", g.QualifiedUsedGoIdent(cfg.DataRespType)))
	}
	if anyNeedValidate(services) {
		ident := validatePackage.Ident("Validate")
		lines = append(lines, fmt.Sprintf("var _ = %s", g.QualifiedUsedGoIdent(ident)))
		fileCfg.packageDesc.ValidateFunc = g.QualifiedUsedGoIdent(ident)
		fileCfg.packageDesc.ValidateErrorFunc = g.QualifiedUsedGoIdent(validateErrorFunc)
	}
	return lines
}

func anyNeedValidate(services []*template.ServiceDesc) bool {
	for _, sd := range services {
		for _, md := range sd.Methods {
			if md.NeedValidate {
				return true
			}
		}
	}
	return false
}
