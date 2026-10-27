package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWorkspace(t *testing.T) {
	dir := filepath.Join("testdata", "work")
	m, err := loadWith(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	var paths []string
	for _, p := range m.Packages {
		paths = append(paths, p.Path)
	}
	want := []string{"example.com/moda", "example.com/modb"}
	if len(paths) != 2 || paths[0] != want[0] || paths[1] != want[1] {
		t.Fatalf("packages = %v, want %v", paths, want)
	}

	// Source positions are re-rooted under the workspace so /source resolves.
	for _, p := range m.Packages {
		for _, tp := range p.Types {
			if !strings.HasPrefix(tp.Pos, p.Path[len("example.com/"):]+"/") {
				t.Errorf("%s.%s pos = %q, want a workspace-relative path", p.Path, tp.Name, tp.Pos)
			}
		}
	}

	// Calls cross the module boundary.
	var found bool
	for _, c := range m.Calls {
		if c.From == "example.com/modb#FB" && c.To == "example.com/moda#FA" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing cross-module call in %v", m.Calls)
	}

	if m.Root == "" || !filepath.IsAbs(m.Root) {
		t.Fatalf("workspace root = %q, want an absolute path", m.Root)
	}
	if len(m.Warnings) != 0 {
		t.Fatalf("warnings = %v", m.Warnings)
	}
}

func TestHasGoWork(t *testing.T) {
	if !hasGoWork(filepath.Join("testdata", "work")) {
		t.Error("testdata/work should be detected as a workspace")
	}
	if hasGoWork(filepath.Join("testdata", "relmod")) {
		t.Error("a plain module must not be detected as a workspace")
	}
	if hasGoWork("") && hasGoWork(".") {
		t.Log("workspace detection is consistent for the repo root")
	}
}

func TestWorkspaceModulesAreSorted(t *testing.T) {
	mods, err := workspaceModules(context.Background(), filepath.Join("testdata", "work"))
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 2 {
		t.Fatalf("modules = %v", mods)
	}
	if !filepath.IsAbs(mods[0]) || !filepath.IsAbs(mods[1]) {
		t.Fatalf("module dirs are not absolute: %v", mods)
	}
	if !strings.HasSuffix(filepath.ToSlash(mods[0]), "moda") {
		t.Fatalf("modules = %v, want moda first", mods)
	}
}
