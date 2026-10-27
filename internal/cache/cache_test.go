package cache

import (
	"context"
	"errors"
	"github.com/Sizolity/archview/internal/model"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func writeSource(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func countingCache(t *testing.T, dir string) (*Cache, *int32) {
	t.Helper()
	var calls int32
	c := New(dir, func(_ context.Context, d string) (*model.Module, error) {
		atomic.AddInt32(&calls, 1)
		return &model.Module{Path: "example.com/mod", Root: d}, nil
	})
	return c, &calls
}

func TestCacheHitDoesNotRescan(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, filepath.Join(dir, "a.go"), "package a\n")
	c, calls := countingCache(t, dir)

	first, err := c.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("scan ran %d times, want 1", got)
	}
	if first != second {
		t.Fatal("cache returned a different model for the same tree")
	}
}

func TestCacheInvalidatesOnChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	writeSource(t, file, "package a\n")
	c, calls := countingCache(t, dir)

	if _, err := c.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	writeSource(t, file, "package a\n\nvar X = 1\n")

	if _, err := c.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("scan ran %d times, want 2", got)
	}
}

func TestCacheInvalidatesOnTouch(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	writeSource(t, file, "package a\n")
	c, calls := countingCache(t, dir)

	if _, err := c.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Same bytes, new timestamp: editors that rewrite a file in place still
	// invalidate the cache.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(file, future, future); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("scan ran %d times, want 2 after a touch", got)
	}
}

func TestFingerprintIgnoresGoExcludedDirs(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, filepath.Join(dir, "a.go"), "package a\n")

	before, err := treeFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}

	// The go command leaves these out of ./..., so they must not invalidate.
	writeSource(t, filepath.Join(dir, "testdata", "x.go"), "package x\n")
	writeSource(t, filepath.Join(dir, ".git", "y.go"), "package y\n")
	writeSource(t, filepath.Join(dir, "_build", "z.go"), "package z\n")
	writeSource(t, filepath.Join(dir, "vendor", "v.go"), "package v\n")

	after, err := treeFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("ignored directories changed the fingerprint")
	}

	// A real source file still does.
	writeSource(t, filepath.Join(dir, "b.go"), "package a\n")
	changed, err := treeFingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if changed == before {
		t.Fatal("a new .go file did not change the fingerprint")
	}
}

func TestCacheDoesNotCacheErrors(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, filepath.Join(dir, "a.go"), "package a\n")

	var calls int32
	boom := errors.New("scan failed")
	c := New(dir, func(_ context.Context, d string) (*model.Module, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return nil, boom
		}
		return &model.Module{Path: "example.com/mod", Root: d}, nil
	})

	if _, err := c.Load(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the loader error", err)
	}
	m, err := c.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		t.Fatal("second load returned no model")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("scan ran %d times, want 2 (errors are not cached)", got)
	}
}
