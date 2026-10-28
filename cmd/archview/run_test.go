package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sizolity/archview/internal/model"
	"github.com/Sizolity/archview/internal/web"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRunVersion(t *testing.T) {
	if err := run(context.Background(), []string{"-version"}); err != nil {
		t.Fatalf("run(-version) = %v", err)
	}
}

// A broken config is a usage error: the CLI documents exit status 2 for it.
func TestRunBadConfigExitsTwo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archview.json")
	if err := os.WriteFile(path, []byte("{ nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run(context.Background(), []string{"-config", path, "-no-crap"})
	if err == nil {
		t.Fatal("expected an error")
	}
	var exit *exitError
	if !errors.As(err, &exit) || exit.code != 2 {
		t.Fatalf("err = %v, want exit code 2", err)
	}
}

func TestRunMissingImport(t *testing.T) {
	err := run(context.Background(), []string{"-import-edn", filepath.Join(t.TempDir(), "nope.edn")})
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRunUnknownFlag(t *testing.T) {
	if err := run(context.Background(), []string{"-nope"}); err == nil {
		t.Fatal("expected an error")
	}
}

// The two export targets are files on disk, so they are checked as files.
func TestWriteOutputs(t *testing.T) {
	dir := t.TempDir()
	m := &model.Module{Packages: []*model.Package{{Path: "p", Name: "p"}}}
	out := filepath.Join(dir, "view.html")
	edn := filepath.Join(dir, "view.edn")

	if err := writeOutputs(m, "source", out, edn, testLogger()); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(page, []byte("<!doctype html")) || !bytes.Contains(page, []byte("__ARCHVIEW_DATA__")) {
		t.Fatalf("page looks wrong: %d bytes", len(page))
	}
	ir, err := os.ReadFile(edn)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(ir, []byte(":packages")) {
		t.Fatalf("EDN looks wrong: %q", ir)
	}
}

// serve must return cleanly when the context is already cancelled, and must
// report a port it cannot bind.
func TestServeStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv := &web.Server{Load: func(context.Context) (*model.Module, error) {
		return &model.Module{}, nil
	}}
	if err := serve(ctx, srv, "127.0.0.1:0", testLogger()); err != nil {
		t.Fatalf("serve = %v", err)
	}
}

func TestServeRefusesBadAddress(t *testing.T) {
	srv := &web.Server{Load: func(context.Context) (*model.Module, error) {
		return &model.Module{}, nil
	}}
	if err := serve(context.Background(), srv, "127.0.0.1:-1", testLogger()); err == nil {
		t.Fatal("expected a listen error")
	}
}
