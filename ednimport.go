package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// This file reads the topology IR back into a model. The reader covers the EDN
// subset an IR uses: maps, vectors, lists, keywords, strings, numbers, booleans
// and nil — enough for archview, not a general EDN implementation.

// ednKeyword is a keyword value; map keys are flattened to plain strings.
type ednKeyword string

type ednParser struct {
	src  []byte
	pos  int
	line int
}

// parseEDN parses one EDN document.
func parseEDN(src []byte) (any, error) {
	p := &ednParser{src: src, line: 1}
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	if err := p.skipSpace(); err != nil {
		return nil, err
	}
	if p.pos < len(p.src) {
		return nil, p.errf("文档在结尾前就结束了")
	}
	return v, nil
}

func (p *ednParser) errf(format string, args ...any) error {
	return fmt.Errorf("EDN 第 %d 行: %s", p.line, fmt.Sprintf(format, args...))
}

func (p *ednParser) skipSpace() error {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case ' ', '\t', '\r', ',':
			p.pos++
		case '\n':
			p.line++
			p.pos++
		case ';':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		default:
			return nil
		}
	}
	return nil
}

func (p *ednParser) value() (any, error) {
	if err := p.skipSpace(); err != nil {
		return nil, err
	}
	if p.pos >= len(p.src) {
		return nil, p.errf("意外的文件结尾")
	}
	switch p.src[p.pos] {
	case '{':
		return p.mapValue()
	case '[':
		return p.seqValue(']')
	case '(':
		return p.seqValue(')')
	case '"':
		return p.stringValue()
	case ':':
		return p.keywordValue()
	default:
		return p.atomValue()
	}
}

func (p *ednParser) mapValue() (map[string]any, error) {
	p.pos++ // {
	out := map[string]any{}
	for {
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.src) {
			return nil, p.errf("map 没有闭合")
		}
		if p.src[p.pos] == '}' {
			p.pos++
			return out, nil
		}
		key, err := p.value()
		if err != nil {
			return nil, err
		}
		val, err := p.value()
		if err != nil {
			return nil, err
		}
		out[ednKey(key)] = val
	}
}

func (p *ednParser) seqValue(close byte) ([]any, error) {
	p.pos++ // [ or (
	out := []any{}
	for {
		if err := p.skipSpace(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.src) {
			return nil, p.errf("序列没有闭合")
		}
		if p.src[p.pos] == close {
			p.pos++
			return out, nil
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

func (p *ednParser) stringValue() (string, error) {
	p.pos++ // opening quote
	var b strings.Builder
	for {
		if p.pos >= len(p.src) {
			return "", p.errf("字符串没有闭合")
		}
		c := p.src[p.pos]
		switch c {
		case '"':
			p.pos++
			return b.String(), nil
		case '\\':
			p.pos++
			if p.pos >= len(p.src) {
				return "", p.errf("转义没有结束")
			}
			switch p.src[p.pos] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			default:
				b.WriteByte(p.src[p.pos])
			}
			p.pos++
		case '\n':
			return "", p.errf("字符串里不能有裸换行")
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
}

func (p *ednParser) keywordValue() (ednKeyword, error) {
	p.pos++ // :
	start := p.pos
	for p.pos < len(p.src) && !ednDelimiter(p.src[p.pos]) {
		p.pos++
	}
	if start == p.pos {
		return "", p.errf("空关键字")
	}
	return ednKeyword(p.src[start:p.pos]), nil
}

func (p *ednParser) atomValue() (any, error) {
	start := p.pos
	for p.pos < len(p.src) && !ednDelimiter(p.src[p.pos]) {
		p.pos++
	}
	word := string(p.src[start:p.pos])
	switch word {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "nil":
		return nil, nil
	}
	if n, err := strconv.ParseFloat(word, 64); err == nil {
		return n, nil
	}
	return nil, p.errf("无法解析的原子 %q", word)
}

func ednDelimiter(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', ',', '{', '}', '[', ']', '(', ')', '"', ';':
		return true
	}
	return false
}

// ednKey flattens a map key: keywords and strings become strings.
func ednKey(v any) string {
	switch k := v.(type) {
	case ednKeyword:
		return string(k)
	case string:
		return k
	case float64:
		return strconv.FormatFloat(k, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

// importEDN builds a model from a uml-viewer topology IR. The second return is
// how many entries were dropped because archview has no place for them.
func importEDN(src []byte) (*Module, int, error) {
	root, err := parseEDN(src)
	if err != nil {
		return nil, 0, err
	}
	top, ok := root.(map[string]any)
	if !ok {
		return nil, 0, fmt.Errorf("EDN 顶层不是 map")
	}

	m := &Module{}
	// The title is the only module-level name the IR carries; keep it so a
	// round trip does not lose it.
	m.Path = ednAsString(top["title"])
	classToType := map[string]string{}
	ignored := 0

	for _, pv := range ednAsList(top["packages"]) {
		pm := ednAsMap(pv)
		if pm == nil {
			ignored++
			continue
		}
		pkgID := ednAsString(pm["id"])
		if pkgID == "" {
			ignored++
			continue
		}
		p := &Package{Path: pkgID, Name: ednAsString(pm["label"])}
		if p.Name == "" {
			p.Name = pkgID
		}

		for _, cv := range ednAsList(pm["classes"]) {
			cm := ednAsMap(cv)
			if cm == nil {
				ignored++
				continue
			}
			classID := ednAsString(cm["id"])
			name := ednAsString(cm["name"])
			if classID == "" || name == "" {
				ignored++
				continue
			}
			t := &TypeInfo{
				ID:       pkgID + "#" + classID,
				Name:     name,
				Kind:     "struct",
				Exported: true,
			}
			if ednAsString(cm["stereotype"]) == "interface" {
				t.Kind = "interface"
			}
			for _, fv := range ednAsList(cm["fields"]) {
				fm := ednAsMap(fv)
				if fm == nil {
					ignored++
					continue
				}
				t.Fields = append(t.Fields, FieldInfo{
					Name:     ednAsString(fm["name"]),
					Type:     ednAsString(fm["type"]),
					Exported: true,
				})
			}
			for _, ov := range ednAsList(cm["ops"]) {
				om := ednAsMap(ov)
				if om == nil {
					ignored++
					continue
				}
				opName := ednAsString(om["name"])
				if opName == "" {
					ignored++
					continue
				}
				args := ednAsStringList(om["args"])
				sig := "()"
				if len(args) > 0 {
					sig = "(" + strings.Join(args, ", ") + ")"
				}
				if ret := ednAsString(om["returns"]); ret != "" {
					sig += " " + ret
				}
				t.Methods = append(t.Methods, MethodInfo{
					ID:       t.ID + "." + opName,
					Name:     opName,
					Sig:      sig,
					Exported: true,
				})
			}

			p.Types = append(p.Types, t)
			classToType[classID] = t.ID
		}
		if len(p.Types) > 0 {
			sort.Slice(p.Types, func(i, j int) bool { return p.Types[i].Name < p.Types[j].Name })
		}
		m.Packages = append(m.Packages, p)
	}

	kinds := map[string]string{
		"association": edgeUses,
		"dependency":  edgeUses,
		"implements":  edgeImplements,
		"aggregation": edgeEmbeds,
		"composition": edgeEmbeds,
		"inheritance": edgeEmbeds,
	}
	for _, ev := range ednAsList(top["edges"]) {
		em := ednAsMap(ev)
		if em == nil {
			ignored++
			continue
		}
		from, okFrom := classToType[ednAsString(em["from"])]
		to, okTo := classToType[ednAsString(em["to"])]
		kind, okKind := kinds[strings.ToLower(ednAsString(em["kind"]))]
		if !okFrom || !okTo || !okKind {
			ignored++
			continue
		}
		m.Edges = append(m.Edges, Edge{Kind: kind, From: from, To: to})
	}

	sort.Slice(m.Packages, func(i, j int) bool { return m.Packages[i].Path < m.Packages[j].Path })
	m.Edges = normalizeEdges(m.Edges)
	return m, ignored, nil
}

func ednAsMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func ednAsList(v any) []any {
	l, _ := v.([]any)
	return l
}

func ednAsString(v any) string {
	switch s := v.(type) {
	case ednKeyword:
		return string(s)
	case string:
		return s
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(s)
	default:
		return ""
	}
}

func ednAsStringList(v any) []string {
	list := ednAsList(v)
	out := make([]string, 0, len(list))
	for _, item := range list {
		out = append(out, ednAsString(item))
	}
	return out
}
