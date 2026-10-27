package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// defaultConfigFile is read from the working directory when -config is absent.
const defaultConfigFile = "archview.json"

// Config is archview.json: the settings that should survive between runs
// without repeating flags. Every field is optional; an absent field means "use
// the command-line default".
type Config struct {
	Exclude []string      `json:"exclude,omitempty"`
	Editor  string        `json:"editor,omitempty"`
	Layers  [][]string    `json:"layers,omitempty"`
	Metrics MetricsConfig `json:"metrics,omitempty"`

	// Path is where the config came from; empty means defaults.
	Path string `json:"-"`
}

// MetricsConfig controls the go-crap step.
type MetricsConfig struct {
	Enabled    *bool `json:"enabled,omitempty"`
	Thresholds []int `json:"thresholds,omitempty"`
}

// loadConfig reads the config file. explicit means -config was passed, so a
// missing file is an error rather than a silent default.
func loadConfig(path string, explicit bool) (*Config, error) {
	if !explicit {
		path = defaultConfigFile
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return &Config{}, nil
			}
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return &Config{Path: path}, nil
	}

	cfg := &Config{}
	dec := json.NewDecoder(bytes.NewReader(data))
	// A typo in a key is a mistake, not a forward-compatible extension.
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	cfg.Path = path
	cfg.normalize()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

func (c *Config) normalize() {
	for i, e := range c.Exclude {
		// Keep a leading "/" so Validate can still reject absolute paths.
		c.Exclude[i] = strings.TrimSuffix(strings.TrimSpace(filepath.ToSlash(e)), "/")
	}
	for i, layer := range c.Layers {
		for j, name := range layer {
			c.Layers[i][j] = strings.Trim(strings.TrimSpace(name), "/")
		}
	}
	if c.Editor != "" {
		c.Editor = strings.TrimSpace(c.Editor)
	}
}

// Validate reports every problem at once: a half-applied config is worse than
// a refusal.
func (c *Config) Validate() error {
	var errs []string
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	if c.Editor != "" && editorMode(c.Editor) != c.Editor {
		add("editor: %q 不是 source|vscode|goland|file|none", c.Editor)
	}

	for i, e := range c.Exclude {
		switch {
		case e == "":
			add("exclude[%d]: 不能为空", i)
		case strings.HasPrefix(e, "/"), strings.HasPrefix(e, ".."),
			strings.Contains(e, "/../"), filepath.VolumeName(e) != "":
			// filepath.IsAbs is not enough: on Windows "/x" is not absolute but
			// it is still not a module-relative path.
			add("exclude[%d]: %q 必须是相对 module root 的路径", i, e)
		}
	}

	if len(c.Layers) > 0 {
		for i, layer := range c.Layers {
			if len(layer) == 0 {
				add("layers[%d]: 不能是空层", i)
				continue
			}
			for j, name := range layer {
				if name == "" {
					add("layers[%d][%d]: 不能为空", i, j)
				}
			}
		}
	}

	if n := len(c.Metrics.Thresholds); n != 0 {
		if n != 2 {
			add("metrics.thresholds: 需要两个整数 [warn, bad],得到 %d 个", n)
		} else if warn, bad := c.Metrics.Thresholds[0], c.Metrics.Thresholds[1]; warn < 0 || bad < warn {
			add("metrics.thresholds: 需要 0 <= warn <= bad,得到 [%d, %d]", warn, bad)
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// configEditor resolves the jump mode: an explicit flag wins, then the config,
// then the built-in source page.
func configEditor(cfg *Config, flagValue string, flagSet bool) string {
	if flagSet {
		return editorMode(flagValue)
	}
	if cfg != nil && cfg.Editor != "" {
		return cfg.Editor
	}
	return "source"
}

// filterPackages drops the packages an exclude entry matches.
func filterPackages(list []listed, modulePath string, exclude []string) []listed {
	if len(exclude) == 0 {
		return list
	}
	kept := make([]listed, 0, len(list))
	for _, lp := range list {
		if excluded(lp.Path, modulePath, exclude) {
			continue
		}
		kept = append(kept, lp)
	}
	return kept
}

// excluded reports whether a package path falls under one of the exclude
// entries. Entries are module-relative path prefixes and match whole segments:
// "internal/gen" hides "internal/gen" and "internal/gen/x" but not
// "internal/genx".
func excluded(pkgPath, modulePath string, entries []string) bool {
	if len(entries) == 0 {
		return false
	}
	rel := pkgPath
	if modulePath != "" {
		if pkgPath == modulePath {
			rel = ""
		} else {
			rel = strings.TrimPrefix(pkgPath, modulePath+"/")
			if rel == pkgPath {
				return false // not inside this module
			}
		}
	}
	if rel == "" {
		return false
	}
	for _, e := range entries {
		if e != "" && (rel == e || strings.HasPrefix(rel, e+"/")) {
			return true
		}
	}
	return false
}
