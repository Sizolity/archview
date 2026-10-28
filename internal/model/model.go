// Package model turns a Go module into the graph the viewer renders: packages,
// types, fields, methods and the relations between them.
//
// The scan is a pipeline — `go list` for the package layout, go/parser for the
// declarations, go/types for what the syntax alone cannot answer (interface
// satisfaction, embedding), and a body walk for the call graph. A package that
// fails to type-check becomes a warning on the model instead of aborting the
// scan, so a module that is mid-edit still renders.
package model

import (
	"bytes"
	"context"
	"fmt"
	"github.com/Sizolity/archview/internal/config"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	gotypes "go/types"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Module is the whole JSON payload the browser reads.
type Module struct {
	Path     string          `json:"path"`
	Root     string          `json:"root,omitempty"`
	Editor   string          `json:"editor,omitempty"`
	Layers   [][]string      `json:"layers,omitempty"`
	Metrics  *MetricsSummary `json:"metrics,omitempty"`
	Packages []*Package      `json:"packages"`
	Edges    []Edge          `json:"edges,omitempty"`
	Calls    []Call          `json:"calls,omitempty"`
	Warnings []string        `json:"warnings,omitempty"`
}

// Package is one Go package with its exported and unexported declarations.
type Package struct {
	Path    string       `json:"path"`
	Name    string       `json:"name"`
	Doc     string       `json:"doc,omitempty"`
	Imports []string     `json:"imports,omitempty"`
	Types   []*TypeInfo  `json:"types,omitempty"`
	Funcs   []*FuncInfo  `json:"funcs,omitempty"`
	Consts  []*ValueInfo `json:"consts,omitempty"`
	Vars    []*ValueInfo `json:"vars,omitempty"`
}

// TypeInfo describes a struct, interface or named type.
type TypeInfo struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Kind     string       `json:"kind"`
	Doc      string       `json:"doc,omitempty"`
	Pos      string       `json:"pos"`
	Exported bool         `json:"exported"`
	Fields   []FieldInfo  `json:"fields,omitempty"`
	Methods  []MethodInfo `json:"methods,omitempty"`
}

// FieldInfo is a struct member. Embedded fields carry the type as Name.
type FieldInfo struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Doc      string   `json:"doc,omitempty"`
	Embedded bool     `json:"embedded,omitempty"`
	Exported bool     `json:"exported"`
	Refs     []string `json:"refs,omitempty"`
	raw      ast.Expr
}

// MethodInfo is a function with a receiver.
type MethodInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Sig      string   `json:"sig"`
	Doc      string   `json:"doc,omitempty"`
	Pos      string   `json:"pos"`
	Exported bool     `json:"exported"`
	Refs     []string `json:"refs,omitempty"`
	Metrics  *Metrics `json:"metrics,omitempty"`
	raw      *ast.FuncType
}

// FuncInfo is a package-level function.
type FuncInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Sig      string   `json:"sig"`
	Doc      string   `json:"doc,omitempty"`
	Pos      string   `json:"pos"`
	Exported bool     `json:"exported"`
	Refs     []string `json:"refs,omitempty"`
	Metrics  *Metrics `json:"metrics,omitempty"`
	raw      *ast.FuncType
}

// ValueInfo is a constant or variable.
type ValueInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Value    string `json:"value,omitempty"`
	Doc      string `json:"doc,omitempty"`
	Exported bool   `json:"exported"`
}

type listed struct {
	Path    string
	Name    string
	Dir     string
	Imports []string
	Files   []string
}

// Load scans the module in the current working directory.
func Load(ctx context.Context) (*Module, error) { return LoadWith(ctx, "", nil) }

// load scans the module rooted at dir; "" means the process working directory.
// Tests point it at a fixture so they never depend on where go test runs.
func LoadDir(ctx context.Context, dir string) (*Module, error) { return LoadWith(ctx, dir, nil) }

// LoadWith scans dir with the config's excludes and layers applied. A go.work
// in dir turns the scan into a workspace scan.
func LoadWith(ctx context.Context, dir string, cfg *config.Config) (*Module, error) {
	if hasGoWork(dir) {
		return loadWorkspace(ctx, dir, cfg)
	}
	return loadModule(ctx, dir, cfg, nil)
}

// loadModule scans one module rooted at dir. scope decides which package paths
// a call may target; nil means "this module only". A workspace passes a
// permissive scope while loading, then filters with the union of its modules.
func loadModule(ctx context.Context, dir string, cfg *config.Config, scope func(string) bool) (*Module, error) {
	modulePath, root, err := moduleInfo(ctx, dir)
	if err != nil {
		return nil, err
	}
	list, err := listPackages(ctx, dir)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	m := &Module{Path: modulePath, Root: root}
	if cfg != nil {
		list = filterPackages(list, modulePath, cfg.Exclude)
		m.Layers = cfg.Layers
		if len(list) == 0 {
			m.Warnings = append(m.Warnings, "exclude 把模块里的所有包都过滤掉了")
		}
	}
	asts := make(map[string][]*ast.File, len(list))
	for _, lp := range list {
		files, err := parseFiles(fset, lp)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", lp.Path, err)
		}
		asts[lp.Path] = files
		m.Packages = append(m.Packages, buildPackage(fset, root, modulePath, lp, files))
	}
	sort.Slice(m.Packages, func(i, j int) bool { return m.Packages[i].Path < m.Packages[j].Path })

	ix := buildIndex(m.Packages)
	var edges []Edge
	for _, p := range m.Packages {
		for _, t := range p.Types {
			for i := range t.Fields {
				t.Fields[i].Refs = resolve(t.Fields[i].raw, p.Path, ix)
				edges = append(edges, useEdges(t.ID, t.Fields[i].Refs)...)
			}
			for i := range t.Methods {
				t.Methods[i].Refs = resolveSig(t.Methods[i].raw, p.Path, ix)
				edges = append(edges, useEdges(t.ID, t.Methods[i].Refs)...)
			}
		}
		for _, f := range p.Funcs {
			f.Refs = resolveSig(f.raw, p.Path, ix)
		}
	}

	checked, warnings := typeCheck(ctx, fset, dir, list, asts)
	m.Warnings = warnings
	if len(checked) > 0 {
		pkgs := make(map[string]*gotypes.Package, len(checked))
		for path, c := range checked {
			pkgs[path] = c.pkg
		}
		edges = append(edges, relations(pkgs, modelIDs(m.Packages))...)
		callScope := scope
		if callScope == nil {
			callScope = func(path string) bool {
				_, ok := checked[path]
				return ok
			}
		}
		m.Calls = collectCalls(checked, callScope)
	}
	m.Edges = NormalizeEdges(edges)
	return m, nil
}

// goCommand builds a go command that runs in dir; "" means the process working
// directory.
func goCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "go", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd
}

func moduleInfo(ctx context.Context, dir string) (path, root string, err error) {
	cmd := goCommand(ctx, dir, "list", "-m", "-f", "{{.Path}}|{{.Dir}}")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("go list -m: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("go list -m: unexpected output %q", out)
	}
	return parts[0], parts[1], nil
}

func listPackages(ctx context.Context, dir string) ([]listed, error) {
	cmd := goCommand(ctx, dir, "list", "-e", "-f",
		`{{.ImportPath}}|{{.Name}}|{{.Dir}}|{{join .GoFiles ","}}|{{join .Imports " "}}`, "./...")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list ./...: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var pkgs []listed
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 || parts[0] == "" || parts[2] == "" {
			continue
		}
		lp := listed{Path: parts[0], Name: parts[1], Dir: parts[2]}
		if parts[3] != "" {
			lp.Files = strings.Split(parts[3], ",")
		}
		if parts[4] != "" {
			lp.Imports = strings.Fields(parts[4])
		}
		pkgs = append(pkgs, lp)
	}
	if len(pkgs) == 0 {
		// go list -e prints a placeholder instead of failing when the module
		// cannot be loaded (for example, an unwritable build cache).
		return nil, fmt.Errorf("go list ./...: no packages; %s", strings.TrimSpace(stderr.String()))
	}
	return pkgs, nil
}

// packageFiles returns the files that belong to the package. go list already
// applied build constraints; the directory fallback keeps parsePackage usable
// from tests without a go list round-trip.
func packageFiles(lp listed) ([]string, error) {
	if len(lp.Files) > 0 {
		paths := make([]string, 0, len(lp.Files))
		for _, name := range lp.Files {
			paths = append(paths, filepath.Join(lp.Dir, name))
		}
		sort.Strings(paths)
		return paths, nil
	}
	entries, err := os.ReadDir(lp.Dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		paths = append(paths, filepath.Join(lp.Dir, name))
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no Go files in %s", lp.Dir)
	}
	return paths, nil
}

type builder struct {
	fset    *token.FileSet
	root    string
	pkgPath string
	rootDoc *Package
	types   map[string]*TypeInfo
	methods map[string][]MethodInfo
}

func parsePackage(fset *token.FileSet, root, module string, lp listed) (*Package, error) {
	files, err := parseFiles(fset, lp)
	if err != nil {
		return nil, err
	}
	return buildPackage(fset, root, module, lp, files), nil
}

// parseFiles parses a package's files in path order, so the model is built the
// same way on every run. Load keeps the ASTs for the type checker.
func parseFiles(fset *token.FileSet, lp listed) ([]*ast.File, error) {
	paths, err := packageFiles(lp)
	if err != nil {
		return nil, err
	}
	files := make([]*ast.File, 0, len(paths))
	for _, path := range paths {
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		if f.Name.Name == lp.Name {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("package %s not found in %s", lp.Name, lp.Dir)
	}
	return files, nil
}

func buildPackage(fset *token.FileSet, root, module string, lp listed, files []*ast.File) *Package {
	p := &Package{Path: lp.Path, Name: lp.Name}
	for _, imp := range lp.Imports {
		if strings.HasPrefix(imp, module+"/") {
			p.Imports = append(p.Imports, imp)
		}
	}
	sort.Strings(p.Imports)

	b := &builder{fset: fset, root: root, pkgPath: lp.Path, rootDoc: p,
		types: map[string]*TypeInfo{}, methods: map[string][]MethodInfo{}}

	for _, f := range files {
		if p.Doc == "" && f.Doc != nil {
			p.Doc = firstPara(f.Doc.Text())
		}
	}
	for _, f := range files {
		b.parseFile(f)
	}
	for name, ms := range b.methods {
		if t, ok := b.types[name]; ok {
			for i := range ms {
				ms[i].ID = methodID(lp.Path, name, ms[i].Name)
			}
			sort.Slice(ms, func(i, j int) bool { return ms[i].Name < ms[j].Name })
			t.Methods = ms
		}
	}
	sort.Slice(p.Types, func(i, j int) bool { return p.Types[i].Name < p.Types[j].Name })
	sort.Slice(p.Funcs, func(i, j int) bool { return p.Funcs[i].Name < p.Funcs[j].Name })
	return p
}

func (b *builder) parseFile(f *ast.File) {
	for _, d := range f.Decls {
		switch dd := d.(type) {
		case *ast.GenDecl:
			switch dd.Tok {
			case token.TYPE:
				for _, spec := range dd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					t := &TypeInfo{
						ID:       b.pkgPath + "#" + ts.Name.Name,
						Name:     ts.Name.Name,
						Kind:     kindOf(ts.Type),
						Doc:      firstPara(commentText(firstNonNil(ts.Doc, dd.Doc))),
						Pos:      b.pos(ts.Pos()),
						Exported: ts.Name.IsExported(),
					}
					if st, ok := ts.Type.(*ast.StructType); ok {
						for _, fld := range st.Fields.List {
							fv := FieldInfo{
								Type:     gotypes.ExprString(fld.Type),
								Doc:      firstPara(commentText(fld.Doc)),
								Exported: isExported(fld.Type),
								raw:      fld.Type,
							}
							if len(fld.Names) == 0 {
								fv.Embedded = true
								fv.Name = fv.Type
								t.Fields = append(t.Fields, fv)
								continue
							}
							for _, nm := range fld.Names {
								one := fv
								one.Name = nm.Name
								one.Exported = nm.IsExported()
								t.Fields = append(t.Fields, one)
							}
						}
					}
					b.rootDoc.Types = append(b.rootDoc.Types, t)
					b.types[t.Name] = t
				}
			case token.CONST, token.VAR:
				for _, spec := range dd.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					doc := firstPara(commentText(firstNonNil(vs.Doc, dd.Doc)))
					tv := ""
					if vs.Type != nil {
						tv = gotypes.ExprString(vs.Type)
					}
					val := ""
					if len(vs.Values) > 0 && dd.Tok == token.CONST {
						val = exprText(b.fset, vs.Values[0])
					}
					for _, nm := range vs.Names {
						v := &ValueInfo{Name: nm.Name, Type: tv, Value: val, Doc: doc, Exported: nm.IsExported()}
						if dd.Tok == token.CONST {
							b.rootDoc.Consts = append(b.rootDoc.Consts, v)
						} else {
							b.rootDoc.Vars = append(b.rootDoc.Vars, v)
						}
					}
				}
			}
		case *ast.FuncDecl:
			if dd.Recv != nil {
				recv := recvName(dd.Recv.List[0].Type)
				b.methods[recv] = append(b.methods[recv], MethodInfo{
					Name:     dd.Name.Name,
					Sig:      funcSig(dd.Type),
					Doc:      firstPara(commentText(dd.Doc)),
					Pos:      b.pos(dd.Pos()),
					Exported: dd.Name.IsExported(),
					raw:      dd.Type,
				})
				continue
			}
			b.rootDoc.Funcs = append(b.rootDoc.Funcs, &FuncInfo{
				ID:       funcID(b.pkgPath, dd.Name.Name),
				Name:     dd.Name.Name,
				Sig:      funcSig(dd.Type),
				Doc:      firstPara(commentText(dd.Doc)),
				Pos:      b.pos(dd.Pos()),
				Exported: dd.Name.IsExported(),
				raw:      dd.Type,
			})
		}
	}
}

func (b *builder) pos(p token.Pos) string {
	pos := b.fset.Position(p)
	rel, err := filepath.Rel(b.root, pos.Filename)
	if err != nil {
		rel = pos.Filename
	}
	return fmt.Sprintf("%s:%d", filepath.ToSlash(rel), pos.Line)
}

func kindOf(e ast.Expr) string {
	switch e.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	default:
		return "type"
	}
}

// isExported reports whether an embedded field's base type is exported.
func isExported(e ast.Expr) bool {
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	switch x := e.(type) {
	case *ast.Ident:
		return x.IsExported()
	case *ast.SelectorExpr:
		return x.Sel.IsExported()
	}
	return true
}

func recvName(e ast.Expr) string {
	if star, ok := e.(*ast.StarExpr); ok {
		e = star.X
	}
	if id, ok := e.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func funcSig(ft *ast.FuncType) string {
	var b strings.Builder
	b.WriteString("(")
	if ft.Params != nil {
		for i, f := range ft.Params.List {
			if i > 0 {
				b.WriteString(", ")
			}
			if len(f.Names) > 0 {
				names := make([]string, 0, len(f.Names))
				for _, n := range f.Names {
					names = append(names, n.Name)
				}
				b.WriteString(strings.Join(names, ", ") + " ")
			}
			b.WriteString(gotypes.ExprString(f.Type))
		}
	}
	b.WriteString(")")
	if ft.Results != nil && len(ft.Results.List) > 0 {
		b.WriteString(" ")
		multi := len(ft.Results.List) > 1
		if multi {
			b.WriteString("(")
		}
		for i, f := range ft.Results.List {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(gotypes.ExprString(f.Type))
		}
		if multi {
			b.WriteString(")")
		}
	}
	return b.String()
}

func exprText(fset *token.FileSet, e ast.Expr) string {
	var b strings.Builder
	if err := printer.Fprint(&b, fset, e); err != nil {
		return ""
	}
	return b.String()
}

func commentText(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	return cg.Text()
}

func firstNonNil(a, b *ast.CommentGroup) *ast.CommentGroup {
	if a != nil {
		return a
	}
	return b
}

func firstPara(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}

type typeIndex struct {
	qualified map[string]string
	byPkg     map[string]map[string]string
}

func buildIndex(pkgs []*Package) *typeIndex {
	ix := &typeIndex{qualified: map[string]string{}, byPkg: map[string]map[string]string{}}
	for _, p := range pkgs {
		ix.byPkg[p.Path] = map[string]string{}
		for _, t := range p.Types {
			ix.byPkg[p.Path][t.Name] = t.ID
			ix.qualified[p.Name+"."+t.Name] = t.ID
		}
	}
	return ix
}

func resolve(expr ast.Expr, pkgPath string, ix *typeIndex) []string {
	if expr == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if p, ok := x.X.(*ast.Ident); ok {
				if id, ok := ix.qualified[p.Name+"."+x.Sel.Name]; ok {
					add(id)
				}
			}
		case *ast.Ident:
			if id, ok := ix.byPkg[pkgPath][x.Name]; ok {
				add(id)
			}
		}
		return true
	})
	sort.Strings(out)
	return out
}

func resolveSig(ft *ast.FuncType, pkgPath string, ix *typeIndex) []string {
	if ft == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(ids []string) {
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	if ft.Params != nil {
		for _, f := range ft.Params.List {
			add(resolve(f.Type, pkgPath, ix))
		}
	}
	if ft.Results != nil {
		for _, f := range ft.Results.List {
			add(resolve(f.Type, pkgPath, ix))
		}
	}
	sort.Strings(out)
	return out
}
