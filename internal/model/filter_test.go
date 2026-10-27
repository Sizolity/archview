package model

import (
	"context"
	"github.com/Sizolity/archview/internal/config"
	"path/filepath"
	"strings"
	"testing"
)

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
	cfg := &config.Config{
		Exclude: []string{"internal/gen"},
		Layers:  [][]string{{"cmd"}, {"web"}},
	}
	m, err := LoadWith(context.Background(), filepath.Join("testdata", "exclude"), cfg)
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
