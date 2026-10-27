package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// modelCache keeps the last scan and rebuilds it only when the source tree
// moved. Fingerprinting costs a directory walk; a scan costs two `go list`
// calls plus a type check, so the walk is worth it.
//
// The model handed out is treated as read-only: the loader applies the editor
// mode and the metrics snapshot before caching, so no request mutates it.
type modelCache struct {
	mu    sync.Mutex
	dir   string
	load  func(context.Context, string) (*Module, error)
	sum   string
	model *Module
}

func newModelCache(dir string, load func(context.Context, string) (*Module, error)) *modelCache {
	return &modelCache{dir: dir, load: load}
}

// Load returns the cached model when the tree is unchanged, otherwise rescans.
func (c *modelCache) Load(ctx context.Context) (*Module, error) {
	sum, err := treeFingerprint(c.dir)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.model != nil && sum == c.sum {
		return c.model, nil
	}
	m, err := c.load(ctx, c.dir)
	if err != nil {
		return nil, err
	}
	c.model = m
	c.sum = sum
	return m, nil
}

// ignoredDir mirrors the directories the go command leaves out of ./...:
// testdata, vendor, and anything starting with "." or "_".
func ignoredDir(name string) bool {
	if name == "testdata" || name == "vendor" {
		return true
	}
	return len(name) > 0 && (name[0] == '.' || name[0] == '_')
}

// treeFingerprint hashes the path, size and mtime of every Go source file plus
// go.mod and go.sum. Unreadable subtrees are skipped: the go command reports
// them with a better message than this walk could.
func treeFingerprint(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	var entries []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir {
				return err
			}
			return nil
		}
		if d.IsDir() {
			if path != dir && ignoredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if name != "go.mod" && name != "go.sum" && !strings.HasSuffix(name, ".go") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			rel = path
		}
		entries = append(entries, rel+"|"+
			strconv.FormatInt(info.ModTime().UnixNano(), 10)+"|"+
			strconv.FormatInt(info.Size(), 10))
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(entries)
	h := sha256.New()
	for _, e := range entries {
		_, _ = h.Write([]byte(e))
		_, _ = h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
