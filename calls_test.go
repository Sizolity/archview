package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCalls(t *testing.T) {
	m, err := load(context.Background(), filepath.Join("testdata", "calls"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Warnings) != 0 {
		t.Fatalf("warnings = %v", m.Warnings)
	}

	want := []Call{
		{From: "example.com/calls#A", To: "example.com/calls#B"},
		{From: "example.com/calls#B", To: "example.com/calls#C"},
		{From: "example.com/calls#Dynamic", To: "example.com/calls#I.M", Dynamic: true},
		{From: "example.com/calls#T.Do", To: "example.com/calls#A"},
	}
	got := make(map[Call]bool, len(m.Calls))
	for _, c := range m.Calls {
		got[c] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing call %+v; got %v", w, m.Calls)
		}
	}
	if len(m.Calls) != len(want) {
		t.Errorf("calls = %v, want the %d above", m.Calls, len(want))
	}

	// Sorted and duplicate-free, so the JSON is diffable.
	for i := 1; i < len(m.Calls); i++ {
		prev, cur := m.Calls[i-1], m.Calls[i]
		if prev.From > cur.From || (prev.From == cur.From && prev.To >= cur.To) {
			t.Fatalf("calls are not sorted: %+v then %+v", prev, cur)
		}
	}
}

func TestCallsUseModelIDs(t *testing.T) {
	m, err := load(context.Background(), filepath.Join("testdata", "calls"))
	if err != nil {
		t.Fatal(err)
	}

	// Every function and method is addressable, which is what the call edges
	// and the inspector both rely on.
	ids := map[string]bool{}
	for _, p := range m.Packages {
		for _, tp := range p.Types {
			for _, mt := range tp.Methods {
				if mt.ID == "" {
					t.Fatalf("method %s.%s has no ID", tp.Name, mt.Name)
				}
				ids[mt.ID] = true
			}
		}
		for _, f := range p.Funcs {
			if f.ID == "" {
				t.Fatalf("func %s has no ID", f.Name)
			}
			ids[f.ID] = true
		}
	}
	for _, c := range m.Calls {
		if !ids[c.From] {
			t.Errorf("call source %q is not a model function", c.From)
		}
		// Static targets must be model functions; dynamic ones resolve to the
		// interface method, which is a label rather than a node.
		if !c.Dynamic && !ids[c.To] {
			t.Errorf("call target %q is not a model function", c.To)
		}
	}
	if owner, member := splitFuncID("example.com/calls#T.Do"); owner != "example.com/calls#T" || member != "Do" {
		t.Errorf("splitFuncID = %q, %q", owner, member)
	}
	if owner, member := splitFuncID("example.com/calls#A"); owner != "" || member != "A" {
		t.Errorf("splitFuncID = %q, %q", owner, member)
	}
}

func TestCallsDegradeWhenTypeCheckFails(t *testing.T) {
	// The broken fixture cannot be checked, so there is no call graph, but the
	// scan still completes.
	m, err := load(context.Background(), filepath.Join("testdata", "broken"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Warnings) == 0 {
		t.Fatal("want a warning from the broken package")
	}
	// Whatever survived must still be valid: no calls from unchecked packages.
	for _, c := range m.Calls {
		if strings.HasPrefix(c.From, "example.com/broken/bad#") {
			t.Errorf("call from an unchecked package: %+v", c)
		}
	}
}
