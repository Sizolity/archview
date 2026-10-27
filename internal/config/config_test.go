package config

import (
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

	cfg, err := Load("", false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path != "" || cfg.Editor != "" || len(cfg.Layers) != 0 {
		t.Fatalf("expected an empty config, got %+v", cfg)
	}
}

func TestLoadConfigExplicitMissingIsAnError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json"), true); err == nil {
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

	cfg, err := Load(path, true)
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
	cfg, err := Load(writeConfig(t, `{"editor": "vscode"}`), true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Editor != "vscode" || len(cfg.Exclude) != 0 || cfg.Metrics.Enabled != nil {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadConfigEmptyFileIsDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, "\n"), true)
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

	_, err := Load(path, true)
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
	_, err := Load(writeConfig(t, `{"metrics": {"thresholds": [30, 5]}}`), true)
	if err == nil || !strings.Contains(err.Error(), "warn <= bad") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadConfigUnknownFieldIsRejected(t *testing.T) {
	_, err := Load(writeConfig(t, `{"editr": "vim"}`), true)
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
		if got := Editor(tc.cfg, tc.flagValue, tc.flagSet); got != tc.want {
			t.Errorf("%s: configEditor = %q, want %q", tc.name, got, tc.want)
		}
	}
}
