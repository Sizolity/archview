package model

import (
	"fmt"
	"go/ast"
	gotypes "go/types"
	"sort"
	"strings"
)

// Call is one edge of the call graph. Both ends are function IDs in the same
// form the model uses: "<pkg>#<Func>" or "<pkg>#<Type>.<Method>". Dynamic means
// the call went through an interface, so the concrete receiver is unknown.
type Call struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Dynamic bool   `json:"dynamic,omitempty"`
}

// collectCalls walks every function body of the packages that type-checked and
// records calls whose target scope accepts the callee's package path. Calls
// into the standard library or a dependency are dropped: they have no node in
// this view.
func collectCalls(checked map[string]*checkedPackage, scope func(pkgPath string) bool) []Call {
	var calls []Call
	for path, cp := range checked {
		for _, file := range cp.files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				from := declFuncID(path, fn)
				if from == "" {
					continue
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					to, dynamic, ok := calleeID(cp.info, call)
					if !ok {
						return true
					}
					if pkgPath, ok := funcIDPackage(to); ok && scope(pkgPath) {
						calls = append(calls, Call{From: from, To: to, Dynamic: dynamic})
					}
					return true
				})
			}
		}
	}
	return normalizeCalls(calls)
}

// funcIDPackage is the package half of a function ID.
func funcIDPackage(id string) (string, bool) {
	i := strings.LastIndex(id, "#")
	if i <= 0 {
		return "", false
	}
	return id[:i], true
}

// declFuncID is the model ID of the function being walked.
func declFuncID(pkgPath string, fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := recvName(fn.Recv.List[0].Type)
		if recv == "" {
			return ""
		}
		return pkgPath + "#" + recv + "." + fn.Name.Name
	}
	return pkgPath + "#" + fn.Name.Name
}

// calleeID resolves the target of a call. A call through a variable (function
// value, field, closure) is not static and is skipped.
func calleeID(info *gotypes.Info, call *ast.CallExpr) (id string, dynamic bool, ok bool) {
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		fn, isFunc := info.Uses[fun].(*gotypes.Func)
		if !isFunc {
			return "", false, false
		}
		id, dynamic = funcObjectID(fn)
		return id, dynamic, id != ""
	case *ast.SelectorExpr:
		if sel, isSel := info.Selections[fun]; isSel {
			fn, isFunc := sel.Obj().(*gotypes.Func)
			if !isFunc {
				return "", false, false
			}
			id, _ = funcObjectID(fn)
			// A method called through an interface has no concrete receiver.
			return id, gotypes.IsInterface(sel.Recv()), id != ""
		}
		if fn, isFunc := info.Uses[fun.Sel].(*gotypes.Func); isFunc {
			id, dynamic = funcObjectID(fn)
			return id, dynamic, id != ""
		}
	}
	return "", false, false
}

// funcObjectID renders a *types.Func as "<pkg>#<Func>" or "<pkg>#<Type>.<Method>".
func funcObjectID(fn *gotypes.Func) (string, bool) {
	pkg := fn.Pkg()
	if pkg == nil {
		return "", false
	}
	pkgPath := pkg.Path()
	sig, isSig := fn.Type().(*gotypes.Signature)
	if !isSig || sig.Recv() == nil {
		return pkgPath + "#" + fn.Name(), false
	}
	recv := gotypes.Unalias(sig.Recv().Type())
	if ptr, isPtr := recv.(*gotypes.Pointer); isPtr {
		recv = gotypes.Unalias(ptr.Elem())
	}
	named, isNamed := recv.(*gotypes.Named)
	if !isNamed {
		return "", false
	}
	_, isIface := named.Underlying().(*gotypes.Interface)
	return pkgPath + "#" + named.Obj().Name() + "." + fn.Name(), isIface
}

// normalizeCalls drops duplicates and sorts, so the JSON is diffable.
func normalizeCalls(calls []Call) []Call {
	seen := make(map[Call]bool, len(calls))
	out := make([]Call, 0, len(calls))
	for _, c := range calls {
		if c.From == "" || c.To == "" || c.From == c.To || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

// SplitFuncID splits "<pkg>#<Type>.<Method>" into the owner type ID and the
// member name; "<pkg>#<Func>" has no owner type.
func SplitFuncID(id string) (owner, member string) {
	i := strings.LastIndex(id, "#")
	if i < 0 {
		return "", id
	}
	rest := id[i+1:]
	if before, after, ok := strings.Cut(rest, "."); ok {
		return id[:i+1] + before, after
	}
	return "", rest
}

// funcID is the model ID for a package-level function.
func funcID(pkgPath, name string) string {
	return fmt.Sprintf("%s#%s", pkgPath, name)
}

// valueID is the model ID for a package-level const or var. The UI already
// addresses value cards by this shape, so the edge can name one directly.
func valueID(pkgPath, name string) string {
	return fmt.Sprintf("%s#%s", pkgPath, name)
}

// methodID is the model ID for a method of a named type.
func methodID(pkgPath, typeName, name string) string {
	return fmt.Sprintf("%s#%s.%s", pkgPath, typeName, name)
}
