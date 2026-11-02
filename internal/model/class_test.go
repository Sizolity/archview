package model

import (
	"sort"
	"testing"
)

// structuralComponents condenses the model's structural edges only, which is
// the input the UI's topology step uses. Data and call edges must not affect
// the result.
func structuralComponents(m *Module) [][]string {
	ids := declarationIDs(m.Packages)
	nodes := make([]string, 0, len(ids))
	for id := range ids {
		nodes = append(nodes, id)
	}
	sort.Strings(nodes)

	adj := make(map[string][]string, len(nodes))
	for _, e := range m.Edges {
		if e.Class != ClassStructural {
			continue
		}
		adj[e.From] = append(adj[e.From], e.To)
	}

	var (
		index   = 0
		order   = map[string]int{}
		low     = map[string]int{}
		onStack = map[string]bool{}
		stack   []string
		comps   [][]string
	)
	var visit func(string)
	visit = func(v string) {
		order[v], low[v] = index, index
		index++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range adj[v] {
			if _, seen := order[w]; !seen {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && order[w] < low[v] {
				low[v] = order[w]
			}
		}
		if low[v] != order[v] {
			return
		}
		var comp []string
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			comp = append(comp, w)
			if w == v {
				break
			}
		}
		sort.Strings(comp)
		comps = append(comps, comp)
	}
	for _, n := range nodes {
		if _, seen := order[n]; !seen {
			visit(n)
		}
	}
	sort.Slice(comps, func(i, j int) bool { return comps[i][0] < comps[j][0] })
	return comps
}

// edgeBetween finds the one relation with the given endpoints; the model
// deduplicates (kind, class, from, to), so there is at most one per direction.
func edgeBetween(t *testing.T, m *Module, from, to string) Edge {
	t.Helper()
	for _, e := range m.Edges {
		if e.From == from && e.To == to {
			return e
		}
	}
	t.Fatalf("缺少边 %s -> %s;实际 %v", from, to, m.Edges)
	return Edge{}
}

func constByName(p *Package, name string) *ValueInfo {
	for _, c := range p.Consts {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// TestTypedConstantsAreDataNotTopology is the bug report: a const names its
// type and a method of that type reads the const back. The two relations are
// data, so the structural-only condensation has no SCC larger than one card
// and the page has no cycle to warn about.
func TestTypedConstantsAreDataNotTopology(t *testing.T) {
	m := loadFixture(t, "severity")
	if len(m.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", m.Warnings)
	}
	const pkg = "example.com/severity"

	// The const's type reference is a data edge.
	e := edgeBetween(t, m, pkg+"#Error", pkg+"#Severity")
	if e.Kind != EdgeUses || e.Class != ClassData {
		t.Errorf("Error -> Severity = %+v, want kind %q class %q", e, EdgeUses, ClassData)
	}
	// The method body reading the const is a data edge too.
	e = edgeBetween(t, m, pkg+"#Severity.String", pkg+"#Error")
	if e.Kind != EdgeValue || e.Class != ClassData {
		t.Errorf("Severity.String -> Error = %+v, want kind %q class %q", e, EdgeValue, ClassData)
	}

	// Structural-only condensation: every declaration is a singleton, and the
	// two cards the bug names are never in one component.
	comps := structuralComponents(m)
	if got, want := len(comps), len(declarationIDs(m.Packages)); got != want {
		t.Fatalf("structural components = %d, want %d singletons (%v)", got, want, comps)
	}
	compOf := map[string]int{}
	for i, comp := range comps {
		if len(comp) != 1 {
			t.Errorf("structural SCC = %v, want every component to be one card", comp)
		}
		compOf[comp[0]] = i
	}
	if compOf[pkg+"#Error"] == compOf[pkg+"#Severity"] {
		t.Errorf("Error and Severity share a structural SCC: %v", comps)
	}

	// The card text is the declaration, not "var Severity = iota".
	errValue := constByName(m.Packages[0], "Error")
	if errValue == nil {
		t.Fatal("missing const Error")
	}
	if errValue.Kind != "const" || errValue.Text != "const Error Severity = iota" {
		t.Errorf("const Error = kind %q text %q, want %q", errValue.Kind, errValue.Text,
			"const Error Severity = iota")
	}
}

// TestMutualTypeReferencesAreStructural is the contrast fixture: two types
// that reference each other are one structural SCC of size two, with a cycle
// on both relation edges.
func TestMutualTypeReferencesAreStructural(t *testing.T) {
	m := loadFixture(t, "mutual")
	if len(m.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", m.Warnings)
	}
	const pkg = "example.com/mutual"

	for _, pair := range [][2]string{
		{pkg + "#A", pkg + "#B"},
		{pkg + "#B", pkg + "#A"},
	} {
		e := edgeBetween(t, m, pair[0], pair[1])
		if e.Kind != EdgeUses || e.Class != ClassStructural {
			t.Errorf("%s -> %s = %+v, want kind %q class %q", pair[0], pair[1], e, EdgeUses, ClassStructural)
		}
	}

	comps := structuralComponents(m)
	if len(comps) != 1 || len(comps[0]) != 2 {
		t.Fatalf("structural components = %v, want one SCC of two cards", comps)
	}
	want := []string{pkg + "#A", pkg + "#B"}
	for i := range want {
		if comps[0][i] != want[i] {
			t.Fatalf("SCC = %v, want %v", comps[0], want)
		}
	}
}

// TestCallsCarryTheCallClass keeps the third value of the class axis honest:
// call edges are in their own list, but they still carry their meaning.
func TestCallsCarryTheCallClass(t *testing.T) {
	m := loadFixture(t, "calls")
	if len(m.Calls) == 0 {
		t.Fatal("fixture has no calls")
	}
	for _, c := range m.Calls {
		if c.Class != ClassCall {
			t.Errorf("call %+v has class %q, want %q", c, c.Class, ClassCall)
		}
	}
}

// TestEveryEdgeHasAVisualKindAndSemanticClass checks the two axes stay
// independent: the kind still says how to draw the line, the class says
// whether topology may use it.
func TestEveryEdgeHasAVisualKindAndSemanticClass(t *testing.T) {
	kinds := map[string]bool{
		EdgeUses: true, EdgeImplements: true, EdgeEmbeds: true, EdgeValue: true,
	}
	for _, name := range []string{"values", "relmod", "severity", "mutual"} {
		m := loadFixture(t, name)
		for _, e := range m.Edges {
			if !kinds[e.Kind] {
				t.Errorf("%s: edge %+v has unknown kind", name, e)
			}
			if e.Class != ClassStructural && e.Class != ClassData {
				t.Errorf("%s: edge %+v has unknown class", name, e)
			}
			if e.Kind == EdgeValue && e.Class != ClassData {
				t.Errorf("%s: value edge %+v must be data, not topology", name, e)
			}
		}
	}
}
