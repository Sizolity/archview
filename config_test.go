package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archview.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfigMissingFallsBackToDefaults(t *testing.T) {
	// -config was not passed and the working directory has no archview.json.
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	cfg, err := loadConfig("", false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path != "" || cfg.Editor != "" || len(cfg.Layers) != 0 {
		t.Fatalf("expected an empty config, got %+v", cfg)
	}
}

func TestLoadConfigExplicitMissingIsAnError(t *testing.T) {
	if _, err := loadConfig(filepath.Join(t.TempDir(), "nope.json"), true); err == nil {
		t.Fatal("want an error for a missing explicit config")
	}
}

func TestLoadConfigFull(t *testing.T) {
	path := writeConfig(t, `{
	  "exclude": ["internal/gen/", "vendor"],
	  "editor": "goland",
	  "layers": [["cmd"], ["web", "wiki"]],
	  "metrics": {"enabled": true, "thresholds": [5, 30]}
	}`)

	cfg, err := loadConfig(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path != path {
		t.Fatalf("path = %q", cfg.Path)
	}
	if got := strings.Join(cfg.Exclude, ","); got != "internal/gen,vendor" {
		t.Fatalf("exclude = %v", cfg.Exclude)
	}
	if cfg.Editor != "goland" || len(cfg.Layers) != 2 || len(cfg.Layers[1]) != 2 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Metrics.Enabled == nil || !*cfg.Metrics.Enabled {
		t.Fatalf("metrics = %+v", cfg.Metrics)
	}
	if len(cfg.Metrics.Thresholds) != 2 || cfg.Metrics.Thresholds[1] != 30 {
		t.Fatalf("thresholds = %v", cfg.Metrics.Thresholds)
	}
}

func TestLoadConfigPartial(t *testing.T) {
	cfg, err := loadConfig(writeConfig(t, `{"editor": "vscode"}`), true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Editor != "vscode" || len(cfg.Exclude) != 0 || cfg.Metrics.Enabled != nil {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadConfigEmptyFileIsDefaults(t *testing.T) {
	cfg, err := loadConfig(writeConfig(t, "\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path == "" || cfg.Editor != "" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

// The acceptance says a bad config reports every problem at once, so a user
// does not fix one field per run.
func TestLoadConfigInvalidReportsEveryProblem(t *testing.T) {
	path := writeConfig(t, `{
	  "exclude": ["", "/abs/path"],
	  "editor": "vim",
	  "layers": [[], ["ok", ""]],
	  "metrics": {"thresholds": [7]}
	}`)

	_, err := loadConfig(path, true)
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	for _, want := range []string{
		`editor: "vim"`,
		"exclude[0]",
		"exclude[1]",
		"layers[0]",
		`layers[1][1]`,
		"metrics.thresholds",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
}

func TestLoadConfigThresholdOrder(t *testing.T) {
	_, err := loadConfig(writeConfig(t, `{"metrics": {"thresholds": [30, 5]}}`), true)
	if err == nil || !strings.Contains(err.Error(), "warn <= bad") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadConfigUnknownFieldIsRejected(t *testing.T) {
	_, err := loadConfig(writeConfig(t, `{"editr": "vim"}`), true)
	if err == nil || !strings.Contains(err.Error(), "editr") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigEditorPrecedence(t *testing.T) {
	cfg := &Config{Editor: "goland"}
	cases := []struct {
		name      string
		cfg       *Config
		flagValue string
		flagSet   bool
		want      string
	}{
		{"flag wins over config", cfg, "vscode", true, "vscode"},
		{"config when no flag", cfg, "source", false, "goland"},
		{"default when neither", &Config{}, "source", false, "source"},
		{"nil config", nil, "", false, "source"},
		{"unknown flag falls back", cfg, "vim", true, "source"},
	}
	for _, tc := range cases {
		if got := configEditor(tc.cfg, tc.flagValue, tc.flagSet); got != tc.want {
			t.Errorf("%s: configEditor = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestExcludedMatchesWholeSegments(t *testing.T) {
	const mod = "example.com/excl"
	cases := []struct {
		pkg  string
		want bool
	}{
		{"example.com/excl", false},
		{"example.com/excl/internal/gen", true},
		{"example.com/excl/internal/gen/deep", true},
		{"example.com/excl/internal/genx", false},
		{"example.com/other/internal/gen", false},
	}
	for _, tc := range cases {
		if got := excluded(tc.pkg, mod, []string{"internal/gen"}); got != tc.want {
			t.Errorf("excluded(%q) = %v, want %v", tc.pkg, got, tc.want)
		}
	}
	if excluded("example.com/excl/internal/gen", mod, nil) {
		t.Error("no entries must exclude nothing")
	}
}

func TestFilterPackages(t *testing.T) {
	list := []listed{
		{Path: "example.com/x"},
		{Path: "example.com/x/internal/gen"},
		{Path: "example.com/x/internal/gen/deep"},
	}
	kept := filterPackages(list, "example.com/x", []string{"internal/gen"})
	if len(kept) != 1 || kept[0].Path != "example.com/x" {
		t.Fatalf("kept = %+v", kept)
	}
	if got := filterPackages(list, "example.com/x", nil); len(got) != 3 {
		t.Fatalf("nil exclude changed the list: %+v", got)
	}
}

func TestLoadWithExcludeAndLayers(t *testing.T) {
	cfg := &Config{
		Exclude: []string{"internal/gen"},
		Layers:  [][]string{{"cmd"}, {"web"}},
	}
	m, err := loadWith(context.Background(), filepath.Join("testdata", "exclude"), cfg)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, p := range m.Packages {
		got = append(got, strings.TrimPrefix(strings.TrimPrefix(p.Path, "example.com/excl"), "/"))
	}
	if len(got) != 2 || got[0] != "" || got[1] != "internal/genx" {
		t.Fatalf("packages = %q, want the root package and internal/genx", got)
	}
	if len(m.Layers) != 2 || m.Layers[0][0] != "cmd" {
		t.Fatalf("layers = %v", m.Layers)
	}
}
