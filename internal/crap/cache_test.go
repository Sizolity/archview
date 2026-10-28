package crap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sizolity/archview/internal/cache"
)

const sampleReport = `{"$schema":"x","version":"1.1.0",` +
	`"summary":{"average":1,"combined":1,"exceeded":0,"total_funcs":1},` +
	`"entries":[{"file":"a.go","function":"A","package":"c","line":3,` +
	`"cyclomatic":1,"coverage":100,"crap":1}]}`

func writeModule(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/c\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package c\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func seedCache(t *testing.T, dir, cacheDir, command string, args []string) {
	t.Helper()
	fp, err := cache.FingerprintOf(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := cacheEntry{Version: cacheVersion, Fingerprint: fp, Args: args, Report: []byte(sampleReport)}
	e.Tool, e.ToolSize, e.ToolMtime = toolIdentity(command)
	writeEntry(cachePath(dir, cacheDir), e)
}

// A cached report is used without running go-crap: the command here does not
// exist, so any attempt to run it would come back as a warning.
func TestCollectCachedSkipsTheTool(t *testing.T) {
	dir, cacheDir := t.TempDir(), t.TempDir()
	writeModule(t, dir)
	const tool = "archview-no-such-tool-xyz"
	seedCache(t, dir, cacheDir, tool, nil)

	b := CollectCached(context.Background(), dir, tool, nil, cacheDir)
	if !b.Cached {
		t.Fatal("expected a cache hit")
	}
	if len(b.Warnings) != 0 {
		t.Fatalf("warnings = %v", b.Warnings)
	}
	if got := b.ByKey["a.go:3"].CRAP; got != 1 {
		t.Fatalf("crap = %v, want 1", got)
	}
}

// Anything the report depends on invalidates it: the tree, the flags and the
// tool binary. A stale entry must not be served.
func TestCollectCachedRejectsStaleEntries(t *testing.T) {
	const tool = "archview-no-such-tool-xyz"
	cases := map[string]func(t *testing.T, dir, cacheDir string){
		"tree changed": func(t *testing.T, dir, cacheDir string) {
			seedCache(t, dir, cacheDir, tool, nil)
			if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package c\n\n// moved\nfunc A() {}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"different flags": func(t *testing.T, dir, cacheDir string) {
			seedCache(t, dir, cacheDir, tool, []string{"--min", "5"})
		},
	}
	for name, prepare := range cases {
		t.Run(name, func(t *testing.T) {
			dir, cacheDir := t.TempDir(), t.TempDir()
			writeModule(t, dir)
			prepare(t, dir, cacheDir)

			b := CollectCached(context.Background(), dir, tool, nil, cacheDir)
			if b.Cached {
				t.Fatal("served a stale entry")
			}
			if len(b.Warnings) == 0 {
				t.Fatal("expected a warning: the tool does not exist")
			}
		})
	}
}

func TestCacheEntryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "crap-x.json")
	e := cacheEntry{
		Version:     cacheVersion,
		Fingerprint: "fp",
		Args:        []string{"--min", "5"},
		Tool:        "go-crap",
		ToolSize:    42,
		ToolMtime:   7,
		Report:      []byte(sampleReport),
	}
	writeEntry(path, e)

	got := readEntry(path, e)
	if string(got) != sampleReport {
		t.Fatalf("report = %q", got)
	}

	e.Fingerprint = "other"
	if readEntry(path, e) != nil {
		t.Fatal("a different fingerprint must miss")
	}
}

func TestCachePathPerModule(t *testing.T) {
	a := cachePath(t.TempDir(), "cache")
	b := cachePath(t.TempDir(), "cache")
	if a == b {
		t.Fatalf("two modules share a cache file: %s", a)
	}
	if filepath.Dir(a) != "cache" {
		t.Fatalf("cache file %s is not in the cache directory", a)
	}
}
