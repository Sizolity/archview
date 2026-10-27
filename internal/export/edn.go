package export

import (
	"fmt"
	"github.com/Sizolity/archview/internal/model"
	"sort"
	"strings"
)

// EDN renders the model as the static topology IR uml-viewer reads:
//
//	{:title ... :direction :tb
//	 :packages [{:id :pkg :label "..." :classes [{:id :type :name "T" ...}]}]
//	 :edges [{:from :a :to :b :kind :association}]}
//
// Relations map onto the edge kinds the viewer knows: uses → :association,
// implements → :implements, embeds → :aggregation. Function-level calls have no
// kind in that IR, so they are left out.
func EDN(m *model.Module) []byte {
	var b strings.Builder
	ids := map[string]string{} // type ID → EDN keyword id
	used := map[string]int{}   // keyword id → how often it was handed out

	keyword := func(base string) string {
		id := base
		if n := used[base]; n > 0 {
			id = fmt.Sprintf("%s-%d", base, n+1)
		}
		used[base]++
		return id
	}

	// Reserve ids first: edges need them, and packages are emitted afterwards.
	for _, p := range m.Packages {
		for _, t := range p.Types {
			ids[t.ID] = keyword(ednSlug(shortName(p.Path), t.Name))
		}
	}

	b.WriteString("{:title " + ednString(moduleTitle(m)) + "\n")
	b.WriteString(" :direction :tb\n")
	b.WriteString(" :packages\n [")
	for i, p := range m.Packages {
		if i > 0 {
			b.WriteString("\n  ")
		}
		b.WriteString(fmt.Sprintf("{:id :%s\n    :label %s\n    :classes\n    [",
			ednSlug(shortName(p.Path)), ednString(pkgLabel(m, p))))
		for j, t := range p.Types {
			if j > 0 {
				b.WriteString("\n     ")
			}
			writeEDNClass(&b, ids[t.ID], t)
		}
		b.WriteString("]}")
	}
	b.WriteString("]\n")

	edges := ednEdges(m, ids)
	b.WriteString(" :edges\n [")
	for i, e := range edges {
		if i > 0 {
			b.WriteString("\n  ")
		}
		b.WriteString(e)
	}
	b.WriteString("]}\n")
	return []byte(b.String())
}

func writeEDNClass(b *strings.Builder, id string, t *model.TypeInfo) {
	stereotype := ":class"
	if t.Kind == "interface" {
		stereotype = ":interface"
	}

	b.WriteString(fmt.Sprintf("{:id :%s\n       :name %s\n       :stereotype %s",
		id, ednString(t.Name), stereotype))

	b.WriteString("\n       :fields [")
	for i, f := range t.Fields {
		if i > 0 {
			b.WriteString(" ")
		}
		name := f.Name
		if f.Embedded {
			// An embedded field has no name of its own; the type is the name.
			name = f.Type
		}
		b.WriteString(fmt.Sprintf("{:name %s :type %s}", ednString(name), ednString(f.Type)))
	}
	b.WriteString("]")

	if crap, cov, ok := typeMetrics(t); ok {
		b.WriteString(fmt.Sprintf("\n       :crap %s", trimFloat(crap)))
		b.WriteString(fmt.Sprintf(" :coverage %s", trimFloat(cov)))
	}

	b.WriteString("\n       :ops [")
	for i, mt := range t.Methods {
		if i > 0 {
			b.WriteString(" ")
		}
		args, returns := splitSig(mt.Sig)
		b.WriteString(fmt.Sprintf("{:name %s :args [%s]", ednString(mt.Name), ednStringList(args)))
		if returns != "" {
			b.WriteString(" :returns " + ednString(returns))
		}
		b.WriteString("}")
	}
	b.WriteString("]}")
}

// typeMetrics averages the method scores: the IR takes one number per class.
func typeMetrics(t *model.TypeInfo) (crap, coverage float64, ok bool) {
	var n int
	for _, mt := range t.Methods {
		if mt.Metrics == nil {
			continue
		}
		crap += mt.Metrics.CRAP
		coverage += mt.Metrics.Coverage / 100 // the IR wants a 0–1 ratio
		n++
	}
	if n == 0 {
		return 0, 0, false
	}
	return crap / float64(n), coverage / float64(n), true
}

// ednEdges renders the relations the IR can express, sorted for a stable file.
func ednEdges(m *model.Module, ids map[string]string) []string {
	kind := map[string]string{
		model.EdgeUses:       ":association",
		model.EdgeImplements: ":implements",
		model.EdgeEmbeds:     ":aggregation",
	}
	var out []string
	for _, e := range m.Edges {
		from, okFrom := ids[e.From]
		to, okTo := ids[e.To]
		k, okKind := kind[e.Kind]
		if !okFrom || !okTo || !okKind {
			continue
		}
		out = append(out, fmt.Sprintf("{:from :%s :to :%s :kind %s}", from, to, k))
	}
	sort.Strings(out)
	return out
}

// splitSig turns " (a string, b int) error" into its arguments and result. It
// only splits on the parenthesis pair and the commas at depth zero, so nested
// parameter types survive.
func splitSig(sig string) (args []string, returns string) {
	s := strings.TrimSpace(sig)
	if !strings.HasPrefix(s, "(") {
		return nil, s
	}
	depth := 0
	end := -1
	for i, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil, s
	}
	inside := s[1:end]
	returns = strings.TrimSpace(s[end+1:])
	if strings.TrimSpace(inside) == "" {
		return nil, returns
	}
	var cur strings.Builder
	depth = 0
	for _, r := range inside {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(cur.String()))
				cur.Reset()
				continue
			}
		}
		cur.WriteRune(r)
	}
	if v := strings.TrimSpace(cur.String()); v != "" {
		args = append(args, v)
	}
	return args, returns
}

func moduleTitle(m *model.Module) string {
	if m.Path != "" {
		return m.Path
	}
	if m.Root != "" {
		return filepathBase(m.Root)
	}
	return "archview"
}

func pkgLabel(m *model.Module, p *model.Package) string {
	if m.Path == "" {
		return p.Path
	}
	rel := strings.TrimPrefix(p.Path, m.Path)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		rel = "."
	}
	return rel
}

func shortName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func filepathBase(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimRight(p, "/")
	return shortName(p)
}

// ednSlug turns a name into a keyword-safe id: lowercase, non-alphanumerics
// collapse to a dash (wiki.Page → wiki-page).
func ednSlug(parts ...string) string {
	joined := strings.ToLower(strings.Join(parts, "-"))
	var b strings.Builder
	dash := false
	for _, r := range joined {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "x"
	}
	return out
}

// ednString quotes an EDN string, escaping what would break the reader.
func ednString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func ednStringList(items []string) string {
	parts := make([]string, 0, len(items))
	for _, s := range items {
		parts = append(parts, ednString(s))
	}
	return strings.Join(parts, " ")
}

// trimFloat prints a number without a trailing ".0".
func trimFloat(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
