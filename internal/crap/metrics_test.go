package crap

import (
	"context"
	"strings"
	"testing"

	"github.com/Sizolity/archview/internal/model"
)

const crapJSON = `{
  "$schema": "https://raw.githubusercontent.com/padiazg/go-crap/master/schemas/report-v1.json",
  "version": "1.1.0",
  "summary": {"average": 12.5, "combined": 25, "exceeded": 1, "total_funcs": 2},
  "entries": [
    {"baseline_crap": 0, "coverage": 0, "delta": 12, "file": "pkg/a.go",
     "function": "A", "package": "a", "crap": 12, "cyclomatic": 3,
     "effective_crap": 12, "line": 10, "mutation_score": 0},
    {"baseline_crap": 0, "coverage": 100, "delta": 1, "file": ".\\pkg\\b.go",
     "function": "B", "package": "b", "crap": 1, "cyclomatic": 1,
     "effective_crap": 1, "line": 20, "mutation_score": 0}
  ]
}`

func TestParseCrapReport(t *testing.T) {
	byKey, summary, err := parseCrapReport([]byte(crapJSON))
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalFuncs != 2 || summary.Exceeded != 1 || summary.Average != 12.5 {
		t.Fatalf("summary = %+v", summary)
	}
	a, ok := byKey["pkg/a.go:10"]
	if !ok {
		t.Fatalf("missing pkg/a.go:10 in %v", byKey)
	}
	if a.Cyclomatic != 3 || a.CRAP != 12 || a.Coverage != 0 {
		t.Fatalf("A = %+v", a)
	}
	// A Windows-style path has to land in the same key space as the model's
	// slash-separated position.
	if _, ok := byKey["pkg/b.go:20"]; !ok {
		t.Fatalf("windows path not normalised: %v", byKey)
	}
}

func TestParseCrapReportRejectsGarbage(t *testing.T) {
	if _, _, err := parseCrapReport([]byte("not json")); err == nil {
		t.Fatal("want an error for a non-JSON report")
	}
}

func TestMetricsApply(t *testing.T) {
	m := &model.Module{Packages: []*model.Package{{
		Path: "example.com/mod",
		Name: "mod",
		Types: []*model.TypeInfo{{
			ID:      "example.com/mod#S",
			Name:    "S",
			Kind:    "struct",
			Pos:     "pkg/a.go:5",
			Methods: []model.MethodInfo{{Name: "M", Pos: "pkg/a.go:10"}},
		}},
		Funcs: []*model.FuncInfo{{Name: "F", Pos: "pkg/b.go:20"}},
	}}}
	b := &Bundle{
		ByKey: map[string]model.Metrics{
			"pkg/a.go:10": {Cyclomatic: 3, CRAP: 12},
			"pkg/b.go:20": {Cyclomatic: 1, CRAP: 1},
		},
		Summary: model.MetricsSummary{TotalFuncs: 2, Exceeded: 1},
	}

	if got := b.Apply(m); got != 2 {
		t.Fatalf("matched = %d, want 2", got)
	}
	if mt := m.Packages[0].Types[0].Methods[0].Metrics; mt == nil || mt.CRAP != 12 {
		t.Fatalf("method metrics = %+v", mt)
	}
	if mt := m.Packages[0].Funcs[0].Metrics; mt == nil || mt.Cyclomatic != 1 {
		t.Fatalf("func metrics = %+v", mt)
	}
	if m.Metrics == nil || m.Metrics.TotalFuncs != 2 {
		t.Fatalf("module summary = %+v", m.Metrics)
	}
}

func TestMetricsApplyLeavesUnknownDeclarationsEmpty(t *testing.T) {
	m := &model.Module{Packages: []*model.Package{{
		Path:  "example.com/mod",
		Name:  "mod",
		Funcs: []*model.FuncInfo{{Name: "F", Pos: "unknown.go:99"}},
	}}}
	b := &Bundle{ByKey: map[string]model.Metrics{"other.go:1": {CRAP: 3}}}

	if got := b.Apply(m); got != 0 {
		t.Fatalf("matched = %d, want 0", got)
	}
	if m.Packages[0].Funcs[0].Metrics != nil {
		t.Fatal("unmatched function got metrics")
	}
	if m.Metrics != nil {
		t.Fatal("summary attached without any match")
	}
}

func TestMetricsApplyCarriesWarnings(t *testing.T) {
	m := &model.Module{}
	b := &Bundle{Warnings: []string{"CRAP 指标不可用: boom"}}
	b.Apply(m)
	if len(m.Warnings) != 1 || !strings.Contains(m.Warnings[0], "CRAP") {
		t.Fatalf("warnings = %v", m.Warnings)
	}
}

func TestCollectMetricsMissingToolDegrades(t *testing.T) {
	b := Collect(context.Background(), "", "archview-no-such-tool-xyz")
	if len(b.Warnings) == 0 {
		t.Fatal("missing tool should produce a warning")
	}
	if !strings.Contains(b.Warnings[0], "CRAP") {
		t.Fatalf("warning = %q", b.Warnings[0])
	}
	if len(b.ByKey) != 0 {
		t.Fatalf("byKey = %v, want empty", b.ByKey)
	}
	// The viewer still serves a model; the warning rides along.
	m := &model.Module{}
	b.Apply(m)
	if len(m.Warnings) != 1 || m.Metrics != nil {
		t.Fatalf("warnings = %v metrics = %+v", m.Warnings, m.Metrics)
	}
}

func TestNormalizePos(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"pkg/a.go:10", "pkg/a.go:10"},
		{"./pkg/a.go:10", "pkg/a.go:10"},
		{"pkg\\a.go:10", "pkg/a.go:10"},
		{"noline", ""},
		{":10", ""},
	} {
		if got := normalizePos(tc.in); got != tc.want {
			t.Errorf("normalizePos(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
