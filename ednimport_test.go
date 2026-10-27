package main

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestParseEDNBasics(t *testing.T) {
	v, err := parseEDN([]byte(`{:a 1 :b [true nil "x"] :c {:d :kw}}`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("top level = %T", v)
	}
	if m["a"] != float64(1) {
		t.Errorf("a = %#v", m["a"])
	}
	list, ok := m["b"].([]any)
	if !ok || len(list) != 3 || list[0] != true || list[1] != nil || list[2] != "x" {
		t.Errorf("b = %#v", m["b"])
	}
	inner := ednAsMap(m["c"])
	if ednAsString(inner["d"]) != "kw" {
		t.Errorf("c.d = %#v", inner["d"])
	}
}

func TestParseEDNStringsAndComments(t *testing.T) {
	src := []byte(";; leading comment\n{:s \"a\\\"b\\nc\"} ; trailing\n")
	v, err := parseEDN(src)
	if err != nil {
		t.Fatal(err)
	}
	if got := ednAsString(ednAsMap(v)["s"]); got != "a\"b\nc" {
		t.Fatalf("string = %q", got)
	}
}

func TestParseEDNRejectsGarbage(t *testing.T) {
	for _, src := range []string{"{", "{:a}", "[1 2", "@nope", `"unterminated`} {
		if _, err := parseEDN([]byte(src)); err == nil {
			t.Errorf("parseEDN(%q) accepted garbage", src)
		}
	}
}

func TestImportEDNMapsTheIR(t *testing.T) {
	src := []byte(`{:title "Demo"
 :direction :tb
 :packages
 [{:id :domain
   :label "Domain"
   :classes
   [{:id :book
     :name "Book"
     :stereotype :class
     :fields [{:name "isbn" :type "String"}]
     :ops [{:name "find" :args ["isbn"] :returns "Book"}]}
    {:id :repo :name "Repo" :stereotype :interface}]}
  {:id :app
   :label "App"
   :classes [{:id :svc :name "Svc"}]}]
 :edges [{:from :repo :to :book :kind :implements}
         {:from :svc :to :book :kind :association}
         {:from :svc :to :nowhere :kind :association}]}`)

	m, ignored, err := importEDN(src)
	if err != nil {
		t.Fatal(err)
	}
	if ignored != 1 {
		t.Errorf("ignored = %d, want 1 (the dangling edge)", ignored)
	}
	if len(m.Packages) != 2 {
		t.Fatalf("packages = %d, want 2", len(m.Packages))
	}
	var domain *Package
	for _, p := range m.Packages {
		if p.Path == "domain" {
			domain = p
		}
	}
	if domain == nil || len(domain.Types) != 2 {
		t.Fatalf("domain package = %+v", domain)
	}

	var book, repo *TypeInfo
	for _, tp := range domain.Types {
		switch tp.Name {
		case "Book":
			book = tp
		case "Repo":
			repo = tp
		}
	}
	if book == nil || repo == nil {
		t.Fatalf("types = %+v", domain.Types)
	}
	if book.Kind != "struct" || repo.Kind != "interface" {
		t.Fatalf("kinds = %s/%s, want struct/interface", book.Kind, repo.Kind)
	}
	if len(book.Fields) != 1 || book.Fields[0].Type != "String" {
		t.Fatalf("fields = %+v", book.Fields)
	}
	if len(book.Methods) != 1 || book.Methods[0].Sig != "(isbn) Book" {
		t.Fatalf("methods = %+v", book.Methods)
	}

	want := Edge{Kind: edgeImplements, From: "domain#repo", To: "domain#book"}
	found := false
	for _, e := range m.Edges {
		if e == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("edges = %+v, want %+v", m.Edges, want)
	}
}

// The acceptance asked for export → import to stay structurally equivalent.
// The IR only carries keyword ids, so the ids themselves differ by design; what
// must survive is the shape: same classes, same members, same relation counts,
// and a second round trip must be a fixed point.
func TestEDNRoundTrip(t *testing.T) {
	original, err := loadWith(context.Background(), filepath.Join("testdata", "relmod"), nil)
	if err != nil {
		t.Fatal(err)
	}

	imported, ignored, err := importEDN(exportEDN(original))
	if err != nil {
		t.Fatal(err)
	}
	if ignored != 0 {
		t.Errorf("import ignored %d entries, want 0", ignored)
	}
	if imported.Path != original.Path {
		t.Errorf("title = %q, want %q", imported.Path, original.Path)
	}

	typeNames := func(m *Module) []string {
		var out []string
		for _, p := range m.Packages {
			for _, tp := range p.Types {
				out = append(out, tp.Name)
			}
		}
		sort.Strings(out)
		return out
	}
	if got, want := typeNames(imported), typeNames(original); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("types = %v, want %v", got, want)
	}

	kindCount := func(m *Module) map[string]int {
		out := map[string]int{}
		for _, e := range m.Edges {
			out[e.Kind]++
		}
		return out
	}
	gotKinds, wantKinds := kindCount(imported), kindCount(original)
	if len(gotKinds) != len(wantKinds) {
		t.Fatalf("edge kinds = %v, want %v", gotKinds, wantKinds)
	}
	for k, n := range wantKinds {
		if gotKinds[k] != n {
			t.Errorf("%s edges = %d, want %d", k, gotKinds[k], n)
		}
	}

	// Method signatures survive the split/rebuild.
	for i, p := range original.Packages {
		for j, tp := range p.Types {
			g := imported.Packages[i].Types[j]
			if len(g.Methods) != len(tp.Methods) {
				t.Fatalf("%s.%s: method count changed", p.Path, tp.Name)
			}
			for k, mt := range tp.Methods {
				if g.Methods[k].Sig != mt.Sig {
					t.Errorf("%s.%s.%s: sig = %q, want %q", p.Path, tp.Name, mt.Name, g.Methods[k].Sig, mt.Sig)
				}
			}
		}
	}

	// A second round trip must be a fixed point, byte for byte.
	again, _, err := importEDN(exportEDN(imported))
	if err != nil {
		t.Fatal(err)
	}
	if first, second := string(exportEDN(imported)), string(exportEDN(again)); first != second {
		t.Fatalf("second round trip changed the document:\n%s\n---\n%s", first, second)
	}
}

func TestImportEDNKeepsSourceEndpointSafe(t *testing.T) {
	// An imported model has no root, so /source must 404 instead of resolving
	// paths against the process working directory.
	m, _, err := importEDN([]byte(`{:packages [{:id :p :classes [{:id :c :name "C"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Root != "" {
		t.Fatalf("imported model root = %q, want empty", m.Root)
	}
	if !strings.Contains(string(exportEDN(m)), "{:id :p-c") {
		t.Fatalf("unexpected export:\n%s", exportEDN(m))
	}
}
