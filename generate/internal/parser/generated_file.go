package parser

import "google.golang.org/protobuf/compiler/protogen"

// GeneratedFile wraps a *protogen.GeneratedFile and records, in a deterministic
// order, every import path referenced through QualifiedGoIdent. The recorded
// idents are later replayed as `var _ = ...` references so that imports which
// would otherwise be unused in the generated file stay alive.
type GeneratedFile struct {
	g       *protogen.GeneratedFile
	imports []string
	dummies map[protogen.GoImportPath]protogen.GoIdent
}

// NewGen returns a GeneratedFile wrapping g.
func NewGen(g *protogen.GeneratedFile) *GeneratedFile {
	return &GeneratedFile{
		g:       g,
		dummies: make(map[protogen.GoImportPath]protogen.GoIdent),
	}
}

// QualifiedGoIdent records id's import path (the first time it is seen) and
// returns the qualified identifier to use in the generated source.
func (g *GeneratedFile) QualifiedGoIdent(id protogen.GoIdent) string {
	if _, ok := g.dummies[id.GoImportPath]; !ok {
		g.imports = append(g.imports, string(id.GoImportPath))
		g.dummies[id.GoImportPath] = id
	}
	return g.g.QualifiedGoIdent(id)
}

// Dummies returns one representative GoIdent per referenced import path, in the
// order the paths were first seen.
func (g *GeneratedFile) Dummies() []protogen.GoIdent {
	idents := make([]protogen.GoIdent, 0, len(g.dummies))
	for _, ident := range g.imports {
		idents = append(idents, g.dummies[protogen.GoImportPath(ident)])
	}
	return idents
}
