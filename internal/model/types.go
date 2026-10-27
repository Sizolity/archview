package model

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/importer"
	"go/token"
	gotypes "go/types"
	"io"
	"os"
	"sort"
	"strings"
)

// Edge kinds. The model uses one flat, sortable list so the browser and the
// future EDN export read the same relations.
const (
	EdgeUses       = "uses"
	EdgeImplements = "implements"
	EdgeEmbeds     = "embeds"
)

// Edge is an explicit relation between two types of the model. Both ends are
// type IDs of the form "<import path>#<TypeName>"; package-level functions are
// not nodes yet, that arrives with the call graph.
type Edge struct {
	Kind string `json:"kind"`
	From string `json:"from"`
	To   string `json:"to"`
}

// exportIndex maps an import path to the export data file that `go list
// -export` produced, so go/types can load dependencies instead of re-parsing
// them. The path comes from the go command, never from a hand-built GOCACHE
// path, so it works the same on Windows and Linux.
type exportIndex map[string]string

func (ix exportIndex) open(path string) (io.ReadCloser, error) {
	file, ok := ix[path]
	if !ok {
		return nil, fmt.Errorf("没有 %q 的导出数据", path)
	}
	return os.Open(file)
}

// listExports asks the go command for export data of the module and of every
// dependency. -e keeps broken packages in the output: they carry no export
// file, and Check reports them package by package instead of failing the scan.
func listExports(ctx context.Context, dir string) (exportIndex, error) {
	cmd := goCommand(ctx, dir, "list", "-e", "-export", "-deps",
		"-f", "{{.ImportPath}}|{{.Export}}", "./...")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -export: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	ix := exportIndex{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		path, file, ok := strings.Cut(line, "|")
		if !ok || path == "" || file == "" {
			continue
		}
		ix[path] = file
	}
	if len(ix) == 0 {
		return nil, fmt.Errorf("go list -export: 没有导出数据")
	}
	return ix, nil
}

// typeCheck type-checks every parsed package and returns the ones that
// succeeded. A failure is a warning, never a fatal error: the model keeps the
// syntactic relations of that package and the server still starts.
func typeCheck(ctx context.Context, fset *token.FileSet, dir string, list []listed, asts map[string][]*ast.File) (map[string]*checkedPackage, []string) {
	ix, err := listExports(ctx, dir)
	if err != nil {
		return nil, []string{"类型检查已跳过,关系只剩语法匹配: " + OneLine(err.Error())}
	}
	conf := &gotypes.Config{Importer: importer.ForCompiler(fset, "gc", ix.open)}

	checked := make(map[string]*checkedPackage, len(list))
	var warnings []string
	for _, lp := range list {
		files := asts[lp.Path]
		if len(files) == 0 {
			continue
		}
		// Uses and Selections are what the call graph needs (2.2): without the
		// type checker there is no way to tell a function call from a method
		// call, or a static call from one through an interface.
		info := &gotypes.Info{
			Uses:       map[*ast.Ident]gotypes.Object{},
			Selections: map[*ast.SelectorExpr]*gotypes.Selection{},
		}
		pkg, err := conf.Check(lp.Path, fset, files, info)
		if err != nil {
			warnings = append(warnings,
				fmt.Sprintf("%s: 类型检查失败,关系降级为语法匹配: %s", lp.Path, OneLine(err.Error())))
			continue
		}
		checked[lp.Path] = &checkedPackage{pkg: pkg, info: info, files: files}
	}
	sort.Strings(warnings)
	return checked, warnings
}

// checkedPackage is one package that type-checked, with what the AST traversal
// needs to resolve identifiers.
type checkedPackage struct {
	pkg   *gotypes.Package
	info  *gotypes.Info
	files []*ast.File
}

// relations turns type-checked packages into implements and embeds edges. Only
// types the model already knows about are linked, so an edge never points at a
// node the browser cannot show.
func relations(checked map[string]*gotypes.Package, known map[string]bool) []Edge {
	nodes := modelTypes(checked, known)
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// An empty interface is satisfied by everything; it would add one edge per
	// type and tell the reader nothing.
	var interfaces []string
	for _, id := range ids {
		if iface, ok := nodes[id].Underlying().(*gotypes.Interface); ok {
			if iface.NumMethods() > 0 || iface.NumEmbeddeds() > 0 {
				interfaces = append(interfaces, id)
			}
		}
	}

	var edges []Edge
	for _, fromID := range ids {
		named := nodes[fromID]
		for _, ifaceID := range interfaces {
			if ifaceID == fromID {
				continue
			}
			iface := nodes[ifaceID].Underlying().(*gotypes.Interface)
			if satisfies(named, iface) {
				edges = append(edges, Edge{Kind: EdgeImplements, From: fromID, To: ifaceID})
			}
		}
	}

	for _, id := range ids {
		st, ok := nodes[id].Underlying().(*gotypes.Struct)
		if !ok {
			continue
		}
		for i := 0; i < st.NumFields(); i++ {
			if !st.Field(i).Anonymous() {
				continue
			}
			if to, ok := moduleNamed(st.Field(i).Type(), nodes); ok && to != id {
				edges = append(edges, Edge{Kind: EdgeEmbeds, From: id, To: to})
			}
		}
	}
	return edges
}

// satisfies reports whether a type meets an interface, counting methods with a
// pointer receiver. A reader asks "does *T implement I" far more often than
// "does the value type T", so both are accepted.
func satisfies(named *gotypes.Named, iface *gotypes.Interface) bool {
	if gotypes.Implements(named, iface) {
		return true
	}
	if _, ok := named.Underlying().(*gotypes.Interface); ok {
		return false // *I is not a valid type
	}
	return gotypes.Implements(gotypes.NewPointer(named), iface)
}

// modelTypes resolves the model's type IDs to go/types declarations, skipping
// generics: their instances are not modelled yet and a bare type parameter
// would produce edges nobody can navigate to.
func modelTypes(checked map[string]*gotypes.Package, known map[string]bool) map[string]*gotypes.Named {
	nodes := map[string]*gotypes.Named{}
	for path, pkg := range checked {
		for _, name := range pkg.Scope().Names() {
			tn, ok := pkg.Scope().Lookup(name).(*gotypes.TypeName)
			if !ok {
				continue
			}
			id := path + "#" + name
			if !known[id] {
				continue
			}
			named, ok := gotypes.Unalias(tn.Type()).(*gotypes.Named)
			if !ok || named.TypeParams().Len() > 0 {
				continue
			}
			nodes[id] = named
		}
	}
	return nodes
}

// moduleNamed resolves an embedded field's type to a model node, dereferencing
// *T and stripping aliases.
func moduleNamed(t gotypes.Type, nodes map[string]*gotypes.Named) (string, bool) {
	t = gotypes.Unalias(t)
	if ptr, ok := t.(*gotypes.Pointer); ok {
		t = gotypes.Unalias(ptr.Elem())
	}
	named, ok := t.(*gotypes.Named)
	if !ok {
		return "", false
	}
	obj := named.Obj()
	if obj.Pkg() == nil {
		return "", false
	}
	id := obj.Pkg().Path() + "#" + obj.Name()
	if _, ok := nodes[id]; !ok {
		return "", false
	}
	return id, true
}

// useEdges turns a resolved reference list into uses edges.
func useEdges(from string, refs []string) []Edge {
	out := make([]Edge, 0, len(refs))
	for _, ref := range refs {
		out = append(out, Edge{Kind: EdgeUses, From: from, To: ref})
	}
	return out
}

// modelIDs is the set of type IDs the browser can navigate to.
func modelIDs(pkgs []*Package) map[string]bool {
	ids := map[string]bool{}
	for _, p := range pkgs {
		for _, t := range p.Types {
			ids[t.ID] = true
		}
	}
	return ids
}

// NormalizeEdges drops self-loops and duplicates and sorts the list, so the
// JSON is diffable and the layout is reproducible.
func NormalizeEdges(edges []Edge) []Edge {
	seen := make(map[Edge]bool, len(edges))
	out := make([]Edge, 0, len(edges))
	for _, e := range edges {
		if e.From == "" || e.To == "" || e.From == e.To || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

// OneLine keeps a multi-line compiler diagnostic on the single line a warning
// field occupies in the JSON.
func OneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
