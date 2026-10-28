package model

import (
	"context"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

const sampleSource = `// Package sample is a test fixture.
package sample

// Page is a page.
type Page struct {
	Title   string
	Kind    Kind
	private int
}

// Kind is a page kind.
type Kind string

// Route returns the route.
func (p Page) Route() string { return "" }

func (p *Page) hidden() {}

// NewPage builds a page.
func NewPage() Page { return Page{} }

// X is a constant.
const X = 1

var Y = 2
`

func parseSample(t *testing.T) *Package {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(sampleSource), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := parsePackage(token.NewFileSet(), dir, "example.com/mod",
		listed{Path: "example.com/mod/sample", Name: "sample", Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParsePackage(t *testing.T) {
	p := parseSample(t)
	if p.Name != "sample" || p.Doc != "Package sample is a test fixture." {
		t.Fatalf("package = %q doc = %q", p.Name, p.Doc)
	}
	if len(p.Types) != 2 {
		t.Fatalf("types = %d, want 2", len(p.Types))
	}

	page := p.Types[1]
	if page.Name != "Page" || page.Kind != "struct" || !page.Exported {
		t.Fatalf("page = %+v", page)
	}
	if len(page.Fields) != 3 {
		t.Fatalf("fields = %d, want 3", len(page.Fields))
	}
	if page.Fields[2].Name != "private" || page.Fields[2].Exported {
		t.Fatalf("private field = %+v", page.Fields[2])
	}
	if len(page.Methods) != 2 || page.Methods[0].Name != "Route" || !page.Methods[0].Exported {
		t.Fatalf("methods = %+v", page.Methods)
	}
	if page.Methods[1].Name != "hidden" || page.Methods[1].Exported {
		t.Fatalf("hidden method = %+v", page.Methods[1])
	}
	if page.Pos == "" {
		t.Fatal("missing position")
	}

	ix := buildIndex([]*Package{p})
	refs := resolve(page.Fields[1].raw, p.Path, ix)
	if len(refs) != 1 || refs[0] != "example.com/mod/sample#Kind" {
		t.Fatalf("refs = %v", refs)
	}

	if len(p.Funcs) != 1 || p.Funcs[0].Name != "NewPage" {
		t.Fatalf("funcs = %+v", p.Funcs)
	}
	if len(p.Consts) != 1 || p.Consts[0].Name != "X" {
		t.Fatalf("consts = %+v", p.Consts)
	}
	if len(p.Vars) != 1 || p.Vars[0].Name != "Y" {
		t.Fatalf("vars = %+v", p.Vars)
	}
}

// A program entry has no exported API; the model has to say so, because the UI
// otherwise draws it as an empty package.
func TestMainPackageIsMarked(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadWith(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Packages) != 1 || !m.Packages[0].Main {
		t.Fatalf("packages = %+v", m.Packages)
	}
}

// Values need a position too: the page opens their source on double-click.
func TestValuePositions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/v\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "package v\n\nconst K = 1\n\nvar X = 2\n"
	if err := os.WriteFile(filepath.Join(dir, "v.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadWith(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := m.Packages[0]
	if len(p.Consts) != 1 || p.Consts[0].Pos != "v.go:3" {
		t.Fatalf("consts = %+v", p.Consts)
	}
	if len(p.Vars) != 1 || p.Vars[0].Pos != "v.go:5" {
		t.Fatalf("vars = %+v", p.Vars)
	}
}
