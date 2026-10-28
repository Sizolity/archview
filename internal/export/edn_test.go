package export

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sizolity/archview/internal/model"
)

func TestExportEDNShape(t *testing.T) {
	m, err := model.LoadWith(context.Background(), filepath.Join("..", "model", "testdata", "relmod"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ir := string(EDN(m))

	for _, want := range []string{
		`{:title "example.com/relmod"`,
		":direction :tb",
		":packages",
		":classes",
		`{:id :relmod-i`,
		`:stereotype :interface`,
		`{:id :relmod-s`,
		":fields",
		":ops",
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("EDN is missing %q\n%s", want, ir)
		}
	}

	// The three relations each map onto an edge kind uml-viewer knows.
	for _, want := range []string{
		"{:from :relmod-s :to :relmod-i :kind :implements}",
		"{:from :relmod-e :to :relmod-s :kind :aggregation}", // embeds
		"{:from :relmod-e :to :relmod-s :kind :association}", // uses
	} {
		if !strings.Contains(ir, want) {
			t.Errorf("EDN is missing edge %q\n%s", want, ir)
		}
	}

	// Every edge endpoint must be a class id in the same document.
	for _, line := range strings.Split(ir, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{:from :") {
			continue
		}
		// "{:from :a :to :b :kind :x}"
		parts := strings.Fields(line)
		if len(parts) < 4 {
			t.Fatalf("malformed edge line %q", line)
		}
		for _, id := range []string{parts[1], parts[3]} {
			id = strings.TrimPrefix(id, ":")
			if !strings.Contains(ir, "{:id :"+id+"\n") {
				t.Errorf("edge endpoint :%s has no class", id)
			}
		}
	}
}

func TestExportEDNEscapesStrings(t *testing.T) {
	m := &model.Module{
		Path: "example.com/q",
		Packages: []*model.Package{{
			Path: "example.com/q",
			Name: "q",
			Types: []*model.TypeInfo{{
				ID:   "example.com/q#T",
				Name: "T",
				Kind: "struct",
				Fields: []model.FieldInfo{
					{Name: "s", Type: `map[string]string`},
					{Name: "quote", Type: `"x"`},
				},
			}},
		}},
	}
	ir := string(EDN(m))
	if !strings.Contains(ir, `:type "\"x\""`) {
		t.Errorf("quotes were not escaped:\n%s", ir)
	}
	if !strings.Contains(ir, `:type "map[string]string"`) {
		t.Errorf("plain type mangled:\n%s", ir)
	}
}

func TestExportEDNIDsAreUnique(t *testing.T) {
	m := &model.Module{
		Path: "example.com/m",
		Packages: []*model.Package{
			{Path: "example.com/a/sub", Name: "sub", Types: []*model.TypeInfo{{ID: "example.com/a/sub#T", Name: "T", Kind: "struct"}}},
			{Path: "example.com/b/sub", Name: "sub", Types: []*model.TypeInfo{{ID: "example.com/b/sub#T", Name: "T", Kind: "struct"}}},
		},
	}
	ir := string(EDN(m))
	if !strings.Contains(ir, "{:id :sub-t\n") {
		t.Fatalf("first id missing:\n%s", ir)
	}
	if !strings.Contains(ir, "{:id :sub-t-2\n") {
		t.Fatalf("collision was not resolved:\n%s", ir)
	}
}

func TestExportEDNMetrics(t *testing.T) {
	m := &model.Module{
		Path: "example.com/m",
		Packages: []*model.Package{{
			Path: "example.com/m",
			Name: "m",
			Types: []*model.TypeInfo{{
				ID:   "example.com/m#S",
				Name: "S",
				Kind: "struct",
				Methods: []model.MethodInfo{
					{Name: "A", Sig: "()", Metrics: &model.Metrics{CRAP: 10, Coverage: 50}},
					{Name: "B", Sig: "()", Metrics: &model.Metrics{CRAP: 20, Coverage: 100}},
				},
			}},
		}},
	}
	ir := string(EDN(m))
	if !strings.Contains(ir, ":crap 15") {
		t.Errorf("mean CRAP missing:\n%s", ir)
	}
	if !strings.Contains(ir, ":coverage 0.75") {
		t.Errorf("mean coverage (0–1) missing:\n%s", ir)
	}
}

func TestSplitSig(t *testing.T) {
	cases := []struct {
		sig     string
		args    []string
		returns string
	}{
		{"()", nil, ""},
		{"() error", nil, "error"},
		{"(a string) error", []string{"a string"}, "error"},
		{"(a string, b int) (int, error)", []string{"a string", "b int"}, "(int, error)"},
		{"(fn func(int) error) bool", []string{"fn func(int) error"}, "bool"},
		{"nonsense", nil, "nonsense"},
	}
	for _, tc := range cases {
		args, returns := splitSig(tc.sig)
		if returns != tc.returns {
			t.Errorf("splitSig(%q) returns = %q, want %q", tc.sig, returns, tc.returns)
		}
		if len(args) != len(tc.args) {
			t.Errorf("splitSig(%q) args = %v, want %v", tc.sig, args, tc.args)
			continue
		}
		for i := range args {
			if args[i] != tc.args[i] {
				t.Errorf("splitSig(%q) arg %d = %q, want %q", tc.sig, i, args[i], tc.args[i])
			}
		}
	}
}

func TestEDNSlug(t *testing.T) {
	cases := map[string]string{
		"wiki.Page":     "wiki-page",
		"Page":          "page",
		"a/b":           "a-b",
		"Store.DB":      "store-db",
		"  weird!! x ":  "weird-x",
		"already-kebab": "already-kebab",
	}
	for in, want := range cases {
		if got := ednSlug(in); got != want {
			t.Errorf("ednSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

// The upstream IR carries visibility as :private on the member, so an exported
// document must say which members the package does not expose.
func TestEDNMarksPrivateMembers(t *testing.T) {
	m := &model.Module{Packages: []*model.Package{{
		Path: "p", Name: "p",
		Types: []*model.TypeInfo{{
			ID: "p#T", Name: "T", Kind: "struct",
			Fields: []model.FieldInfo{
				{Name: "Pub", Type: "int", Exported: true},
				{Name: "priv", Type: "int"},
			},
			Methods: []model.MethodInfo{
				{Name: "Pub", Sig: "()", Exported: true},
				{Name: "priv", Sig: "()"},
			},
		}},
	}}}
	doc := string(EDN(m))
	if n := strings.Count(doc, ":private true"); n != 2 {
		t.Fatalf(":private appears %d times, want 2:\n%s", n, doc)
	}

	back, _, err := Import([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	typ := back.Packages[0].Types[0]
	if typ.Fields[0].Exported != true || typ.Fields[1].Exported != false {
		t.Fatalf("fields = %+v", typ.Fields)
	}
	if typ.Methods[0].Exported != true || typ.Methods[1].Exported != false {
		t.Fatalf("methods = %+v", typ.Methods)
	}
}
