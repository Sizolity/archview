package model

import (
	"go/ast"
	"go/token"
	gotypes "go/types"
)

// referenceEdges turns the identifiers a declaration mentions into edges to
// the declarations they resolve to. The name matching the syntactic pass does
// cannot see through shadowing or distinguish a local from a package-level
// value, so every lookup goes through go/types' Uses table: a parameter, a
// field or a local variable resolves to an object that is not declared at
// package scope and is dropped.
//
// Functions and methods contribute their signature types and their whole body;
// a type declaration contributes its base type and field types; a value
// declaration contributes its type and initializer. Methods are separate
// declarations with their own IDs, so a method body is not attributed to the
// type card.
func referenceEdges(checked map[string]*checkedPackage, pkgs []*Package) []Edge {
	known := declarationIDs(pkgs)
	var edges []Edge
	for path, cp := range checked {
		for _, file := range cp.files {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.GenDecl:
					edges = append(edges, valueOrTypeEdges(path, cp.info, known, d)...)
				case *ast.FuncDecl:
					from := declFuncID(path, d)
					if !known[from] {
						continue
					}
					edges = append(edges, declEdges(cp.info, known, from, d.Type, d.Body)...)
				}
			}
		}
	}
	return edges
}

// valueOrTypeEdges handles the two GenDecl forms the model tracks. A type
// declaration's methods are separate FuncDecls and are handled by the caller.
func valueOrTypeEdges(path string, info *gotypes.Info, known map[string]bool, d *ast.GenDecl) []Edge {
	var edges []Edge
	switch d.Tok {
	case token.TYPE:
		for _, spec := range d.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			from := path + "#" + ts.Name.Name
			if !known[from] {
				continue
			}
			edges = append(edges, declEdges(info, known, from, ts.Type)...)
		}
	case token.CONST, token.VAR:
		for _, spec := range d.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				from := valueID(path, name.Name)
				if !known[from] {
					continue
				}
				nodes := make([]ast.Node, 0, len(vs.Values)+1)
				if vs.Type != nil {
					nodes = append(nodes, vs.Type)
				}
				for _, value := range vs.Values {
					nodes = append(nodes, value)
				}
				edges = append(edges, declEdges(info, known, from, nodes...)...)
			}
		}
	}
	return edges
}

// declEdges resolves every identifier under nodes to a package-level model
// declaration. from is the declaration that mentions them; the returned edges
// are already deduplicated and never point at from itself.
func declEdges(info *gotypes.Info, known map[string]bool, from string, nodes ...ast.Node) []Edge {
	seen := map[Edge]bool{}
	var edges []Edge
	add := func(obj gotypes.Object) {
		id, kind, ok := referenceTarget(obj, known)
		if !ok || id == from {
			return
		}
		edge := Edge{Kind: kind, From: from, To: id}
		if seen[edge] {
			return
		}
		seen[edge] = true
		edges = append(edges, edge)
	}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		ast.Inspect(node, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok {
				add(info.Uses[ident])
			}
			return true
		})
	}
	return edges
}

// referenceTarget maps a go/types object to the declaration ID the model uses,
// keeping only types and package-level constants/variables that are actually
// part of the model. A field, parameter, local, function or builtin yields no
// edge: the first three are not declarations the graph can show, and calls
// already have their own edge list.
func referenceTarget(obj gotypes.Object, known map[string]bool) (id, kind string, ok bool) {
	if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
		return "", "", false
	}
	switch obj := obj.(type) {
	case *gotypes.TypeName:
		id, kind = obj.Pkg().Path()+"#"+obj.Name(), EdgeUses
	case *gotypes.Const:
		id, kind = valueID(obj.Pkg().Path(), obj.Name()), EdgeValue
	case *gotypes.Var:
		id, kind = valueID(obj.Pkg().Path(), obj.Name()), EdgeValue
	default:
		return "", "", false
	}
	if !known[id] {
		return "", "", false
	}
	return id, kind, true
}

// declarationIDs is the set of every declaration the model exposes: types,
// methods, package-level functions, constants and variables. referenceEdges
// uses it on both ends so an excluded package or a declaration the parser kept
// but the model dropped never becomes a dangling edge.
func declarationIDs(pkgs []*Package) map[string]bool {
	ids := map[string]bool{}
	for _, p := range pkgs {
		for _, t := range p.Types {
			ids[t.ID] = true
			for _, m := range t.Methods {
				if m.ID != "" {
					ids[m.ID] = true
				}
			}
		}
		for _, f := range p.Funcs {
			if f.ID != "" {
				ids[f.ID] = true
			}
		}
		for _, v := range p.Consts {
			ids[v.ID] = true
		}
		for _, v := range p.Vars {
			ids[v.ID] = true
		}
	}
	return ids
}
