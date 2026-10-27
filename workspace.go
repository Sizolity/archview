package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// hasGoWork reports whether dir is a workspace root.
func hasGoWork(dir string) bool {
	_, err := os.Stat(filepath.Join(orDot(dir), "go.work"))
	return err == nil
}

func orDot(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}

// workspaceModules lists the module directories a go.work points at, sorted so
// the merged model is deterministic.
func workspaceModules(ctx context.Context, dir string) ([]string, error) {
	out, err := goCommand(ctx, dir, "work", "edit", "-json").Output()
	if err != nil {
		return nil, fmt.Errorf("go work edit -json: %w", err)
	}
	var wf struct {
		Use []struct {
			DiskPath string
		}
	}
	if err := json.Unmarshal(out, &wf); err != nil {
		return nil, fmt.Errorf("go work edit -json: %w", err)
	}
	if len(wf.Use) == 0 {
		return nil, fmt.Errorf("go.work 里没有 use 指令")
	}

	base := orDot(dir)
	dirs := make([]string, 0, len(wf.Use))
	for _, use := range wf.Use {
		p := use.DiskPath
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, abs)
	}
	sort.Strings(dirs)
	return dirs, nil
}

// loadWorkspace scans every module of a go.work and merges the results into one
// model. Package paths stay unique because they are import paths; source
// positions gain the module's directory relative to the workspace root, so
// /source can still resolve a file.
func loadWorkspace(ctx context.Context, dir string, cfg *Config) (*Module, error) {
	root, err := filepath.Abs(orDot(dir))
	if err != nil {
		return nil, err
	}
	mods, err := workspaceModules(ctx, dir)
	if err != nil {
		return nil, err
	}

	ws := &Module{Root: root}
	var loaded []*Module
	for _, modDir := range mods {
		// Keep every call while loading: a target in another workspace module
		// is valid, and the union of package paths is only known afterwards.
		m, err := loadModule(ctx, modDir, cfg, func(string) bool { return true })
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, m)
	}

	known := map[string]bool{}
	for _, m := range loaded {
		for _, p := range m.Packages {
			known[p.Path] = true
		}
	}

	for _, m := range loaded {
		prefix, err := filepath.Rel(root, m.Root)
		if err != nil || prefix == "." {
			prefix = ""
		}
		for _, p := range m.Packages {
			reRootPackage(p, prefix)
			ws.Packages = append(ws.Packages, p)
		}
		ws.Edges = append(ws.Edges, m.Edges...)
		for _, c := range m.Calls {
			if pkgPath, ok := funcIDPackage(c.To); ok && known[pkgPath] {
				ws.Calls = append(ws.Calls, c)
			}
		}
		ws.Warnings = append(ws.Warnings, m.Warnings...)
	}

	sort.Slice(ws.Packages, func(i, j int) bool { return ws.Packages[i].Path < ws.Packages[j].Path })
	ws.Edges = normalizeEdges(ws.Edges)
	ws.Calls = normalizeCalls(ws.Calls)
	if cfg != nil {
		ws.Layers = cfg.Layers
	}
	return ws, nil
}

// reRootPackage moves a package's source positions under the workspace root.
func reRootPackage(p *Package, prefix string) {
	if prefix == "" {
		return
	}
	shift := func(pos string) string {
		if pos == "" {
			return pos
		}
		return filepath.ToSlash(filepath.Join(prefix, pos))
	}
	for _, t := range p.Types {
		t.Pos = shift(t.Pos)
		for i := range t.Methods {
			t.Methods[i].Pos = shift(t.Methods[i].Pos)
		}
	}
	for _, f := range p.Funcs {
		f.Pos = shift(f.Pos)
	}
}
