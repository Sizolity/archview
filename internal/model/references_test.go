package model

import "testing"

// TestReferenceEdgesCoverValuesAndTypes is the fixture from the bug report: a
// function and a method that read package-level values, and functions whose
// parameter or result mentions a local type. None of these were edges before
// the go/types reference pass.
func TestReferenceEdgesCoverValuesAndTypes(t *testing.T) {
	m := loadFixture(t, "values")
	if len(m.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", m.Warnings)
	}

	const pkg = "example.com/values"
	want := []Edge{
		{Kind: EdgeValue, From: pkg + "#v", To: pkg + "#k"},
		{Kind: EdgeValue, From: pkg + "#f", To: pkg + "#k"},
		{Kind: EdgeValue, From: pkg + "#f", To: pkg + "#v"},
		{Kind: EdgeUses, From: pkg + "#f", To: pkg + "#T"},
		{Kind: EdgeUses, From: pkg + "#g", To: pkg + "#T"},
		{Kind: EdgeValue, From: pkg + "#T.m", To: pkg + "#k"},
	}
	set := edgeSet(m)
	for _, w := range want {
		if !set[w] {
			t.Errorf("缺少边 %+v,实际 %v", w, m.Edges)
		}
	}
	if len(m.Edges) != len(want) {
		t.Errorf("edges = %v, want exactly %d above", m.Edges, len(want))
	}

	// The model keeps unexported targets; the UI's "含未导出" filter, not the
	// scanner, decides whether they are drawn.
	p := m.Packages[0]
	var k *ValueInfo
	for _, c := range p.Consts {
		if c.Name == "k" {
			k = c
		}
	}
	if k == nil || k.Exported || k.ID != pkg+"#k" {
		t.Fatalf("const k = %+v, want an unexported, addressable declaration", k)
	}
	if len(p.Vars) != 1 || p.Vars[0].Exported || p.Vars[0].ID != pkg+"#v" {
		t.Fatalf("vars = %+v, want an unexported, addressable v", p.Vars)
	}

	// No self-edges and no duplicate (from, to) pair, whatever the kind.
	seen := map[Edge]bool{}
	for _, e := range m.Edges {
		if e.From == e.To {
			t.Errorf("自环边: %+v", e)
		}
		if seen[e] {
			t.Errorf("重复边: %+v", e)
		}
		seen[e] = true
	}

	// The pass stays honest about what it calls a declaration: the field
	// access x.n and the parameter x are not nodes.
	for _, e := range m.Edges {
		if e.To == pkg+"#n" || e.To == pkg+"#x" {
			t.Errorf("edge points at a local object: %+v", e)
		}
	}
}
