package http

import (
	"fmt"

	"github.com/go-sphere/protoc-gen-sphere/generate/internal/parser"
	"google.golang.org/protobuf/compiler/protogen"
)

const validatePackage = protogen.GoImportPath("buf.build/go/protovalidate")

// collectGoImport emits the `var _ = ...` lines that keep referenced-but-unused
// imports alive in the generated file, and, when any request message needs
// validation, wires up the validate func on the package descriptor.
func collectGoImport(file *protogen.File, g *parser.GeneratedFile, cfg *Config, fileCfg *fileConfig) []string {
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
		lines = append(lines, fmt.Sprintf("var _ = %s[any]", g.QualifiedGoIdent(cfg.ServerHandlerFunc)))
	}
	if !didImport[cfg.DataRespType.GoImportPath] {
		didImport[cfg.DataRespType.GoImportPath] = true
		lines = append(lines, fmt.Sprintf("var _ = new(%s[any])", g.QualifiedGoIdent(cfg.DataRespType)))
	}
LOOP:
	for _, service := range file.Services {
		for _, method := range service.Methods {
			if requestNeedsValidate(method.Input) {
				ident := validatePackage.Ident("Validate")
				lines = append(lines, fmt.Sprintf("var _ = %s", g.QualifiedGoIdent(ident)))
				fileCfg.packageDesc.ValidateFunc = g.QualifiedGoIdent(ident)
				break LOOP
			}
		}
	}
	return lines
}
