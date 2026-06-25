package http

import (
	"fmt"
	"slices"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/parser"
	"google.golang.org/protobuf/compiler/protogen"
)

const validatePackage = protogen.GoImportPath("buf.build/go/protovalidate")

// collectGoImport emits the `var _ = ...` lines that keep referenced-but-unused
// imports alive in the generated file, and, when any request message needs
// validation, wires up the validate func on the package descriptor.
func collectGoImport(file *protogen.File, gen *parser.GeneratedFile, conf *Config, genConf *genConfig) []string {
	lines := make([]string, 0)
	didImport := make(map[protogen.GoImportPath]bool)
	didImport[file.GoImportPath] = true
	for _, ident := range gen.Dummies() {
		if !didImport[ident.GoImportPath] {
			didImport[ident.GoImportPath] = true
			lines = append(lines, fmt.Sprintf("var _  = (*%s)(nil)", gen.QualifiedGoIdent(ident)))
		}
	}
	if !didImport[conf.ServerHandlerFunc.GoImportPath] {
		didImport[conf.ServerHandlerFunc.GoImportPath] = true
		lines = append(lines, fmt.Sprintf("var _  = %s[any]", gen.QualifiedGoIdent(conf.ServerHandlerFunc)))
	}
	if !didImport[conf.DataRespType.GoImportPath] {
		didImport[conf.DataRespType.GoImportPath] = true
		lines = append(lines, fmt.Sprintf("var _  = (*%s[any])(nil)", gen.QualifiedGoIdent(conf.DataRespType)))
	}
LOOP:
	for _, service := range file.Services {
		for _, method := range service.Methods {
			if hasValidateOptionsInMessage(method.Input) || slices.ContainsFunc(method.Input.Fields, hasValidateOptions) {
				ident := validatePackage.Ident("Validate")
				lines = append(lines, fmt.Sprintf("var _ = %s", gen.QualifiedGoIdent(ident)))
				genConf.packageDesc.ValidateFunc = gen.QualifiedGoIdent(ident)
				break LOOP
			}
		}
	}
	return lines
}
