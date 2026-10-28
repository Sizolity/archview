package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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
