package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sizolity/archview/internal/config"
	"github.com/Sizolity/archview/internal/crap"
	"github.com/Sizolity/archview/internal/model"
)

// A reload rebuilds the model, so the CRAP snapshot has to be rebuilt with it.
// go-crap keys its report by file:line; a snapshot older than the model loses
// almost every score as soon as one line moves.
func TestLoadModelRecollectsMetrics(t *testing.T) {
	dir := t.TempDir()
	write := func(src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/t\n\ngo 1.22\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("package t\n\nfunc A() int { return 1 }\n")

	calls := 0
	collect := func(context.Context, string) *crap.Bundle {
		calls++
		return &crap.Bundle{
			ByKey:   map[string]model.Metrics{"a.go:3": {Cyclomatic: 1, CRAP: 1}},
			Summary: model.MetricsSummary{TotalFuncs: 1},
		}
	}
	cfg := &config.Config{}

	first, err := loadModel(context.Background(), dir, cfg, "source", collect)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("collect calls = %d, want 1", calls)
	}
	if got := first.Packages[0].Funcs[0]; got.Metrics == nil || got.Metrics.CRAP != 1 {
		t.Fatalf("first load lost the score: %+v", got.Metrics)
	}

	// A comment shifts the function down one line. A stale snapshot would leave
	// it unmatched; the loader must ask for a fresh one.
	write("package t\n\n// A is a function.\nfunc A() int { return 1 }\n")
	recollect := func(context.Context, string) *crap.Bundle {
		calls++
		return &crap.Bundle{
			ByKey:   map[string]model.Metrics{"a.go:4": {Cyclomatic: 1, CRAP: 2}},
			Summary: model.MetricsSummary{TotalFuncs: 1},
		}
	}

	second, err := loadModel(context.Background(), dir, cfg, "source", recollect)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("collect calls = %d, want 2 after a reload", calls)
	}
	if got := second.Packages[0].Funcs[0]; got.Metrics == nil || got.Metrics.CRAP != 2 {
		t.Fatalf("reload lost the score: %+v", got.Metrics)
	}
}

// With collection switched off the model still loads, with no scores and no
// warnings about it.
func TestLoadModelWithoutMetrics(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/t\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package t\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := loadModel(context.Background(), dir, &config.Config{}, "source", nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Metrics != nil || len(m.Warnings) != 0 {
		t.Fatalf("metrics = %+v warnings = %v", m.Metrics, m.Warnings)
	}
}
