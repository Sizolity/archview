package model

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) *Module {
	t.Helper()
	m, err := LoadDir(context.Background(), filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return m
}

func edgeSet(m *Module) map[Edge]bool {
	set := make(map[Edge]bool, len(m.Edges))
	for _, e := range m.Edges {
		set[e] = true
	}
	return set
}

func TestLoadRelations(t *testing.T) {
	m := loadFixture(t, "relmod")
	if len(m.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", m.Warnings)
	}

	set := edgeSet(m)
	for _, want := range []Edge{
		// Value receiver.
		{Kind: EdgeImplements, From: "example.com/relmod#S", To: "example.com/relmod#I"},
		// Only *P has the method, but a reader still counts P as satisfying I.
		{Kind: EdgeImplements, From: "example.com/relmod#P", To: "example.com/relmod#I"},
		// Promotion through the embedded field makes E satisfy I too.
		{Kind: EdgeImplements, From: "example.com/relmod#E", To: "example.com/relmod#I"},
		// Cross-package relation.
		{Kind: EdgeImplements, From: "example.com/relmod#S", To: "example.com/relmod/sub#J"},
		// Struct embedding, plus the syntactic uses edge it also produces.
		{Kind: EdgeEmbeds, From: "example.com/relmod#E", To: "example.com/relmod#S"},
		{Kind: EdgeUses, From: "example.com/relmod#E", To: "example.com/relmod#S"},
	} {
		if !set[want] {
			t.Errorf("缺少边 %+v,实际 %v", want, m.Edges)
		}
	}

	// Every endpoint must be a node the browser can navigate to.
	ids := modelIDs(m.Packages)
	for _, e := range m.Edges {
		if !ids[e.From] || !ids[e.To] {
			t.Errorf("边指向未知节点: %+v", e)
		}
		if e.From == e.To {
			t.Errorf("自环边: %+v", e)
		}
	}

	// Same input, same output: the JSON is diffable and the layout reproducible.
	again := loadFixture(t, "relmod")
	if len(again.Edges) != len(m.Edges) {
		t.Fatalf("两次扫描边数不同: %d vs %d", len(m.Edges), len(again.Edges))
	}
	for i := range m.Edges {
		if again.Edges[i] != m.Edges[i] {
			t.Fatalf("边顺序不稳定: %+v vs %+v", m.Edges[i], again.Edges[i])
		}
	}
}

func TestLoadDegradesOnTypeError(t *testing.T) {
	m := loadFixture(t, "broken")
	joined := strings.Join(m.Warnings, "\n")
	if !strings.Contains(joined, "example.com/broken/bad") {
		t.Fatalf("warnings 未指出失败的包: %v", m.Warnings)
	}
	if strings.Contains(joined, "example.com/broken/good") {
		t.Fatalf("类型正确的包不应产生警告: %v", m.Warnings)
	}

	// The scan still completes: both packages are in the model and the broken
	// one keeps its syntactic declarations.
	if len(m.Packages) != 2 {
		t.Fatalf("packages = %d, want 2", len(m.Packages))
	}
	for _, p := range m.Packages {
		if len(p.Types) == 0 {
			t.Errorf("%s: 类型为空,解析阶段应仍然可用", p.Path)
		}
	}
}
