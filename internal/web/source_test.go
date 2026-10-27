package web

import (
	"strings"
	"testing"
)

// Every line of the file must survive rendering, markup or not. This is the
// property that matters: highlighting may only add spans, never lose text.
func assertLinesPreserved(t *testing.T, src []byte, page string) {
	t.Helper()
	plain := stripTags(page)
	for _, line := range strings.Split(string(src), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.Contains(plain, line) {
			t.Errorf("line %q is missing from the rendered page", line)
		}
	}
}

func TestRenderSourceHighlightsGo(t *testing.T) {
	src := []byte("package a\n\n// doc\nfunc F(x int) string {\n\treturn \"hi\" // tail\n}\n")
	page := string(renderSource("a.go", src, "/", nil))

	for _, want := range []string{
		`<span class="kw">package</span>`,
		`<span class="kw">func</span>`,
		`<span class="kw">return</span>`,
		`<span class="com">// doc</span>`,
		`<span class="str">&#34;hi&#34;</span>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q in the rendered page", want)
		}
	}
	assertLinesPreserved(t, src, page)
}

// A raw string that spans lines is the case that used to read as "leaked CSS":
// each line must still be its own row, still inside the string span.
func TestRenderSourceSplitsMultiLineLiterals(t *testing.T) {
	src := []byte("package a\n\nconst css = `a{\n  b: c;\n}`\n")
	page := string(renderSource("a.go", src, "/", nil))

	if !strings.Contains(page, `<span class="str">`) {
		t.Fatalf("no string span in:\n%s", page)
	}
	if !strings.Contains(page, `id="L3"`) || !strings.Contains(page, `id="L5"`) {
		t.Fatalf("multi-line literal collapsed a line:\n%s", page)
	}
	assertLinesPreserved(t, src, page)
}

func TestRenderSourceCountsLines(t *testing.T) {
	cases := map[string]string{
		"package a\n":            "1 行",
		"package a":              "1 行",
		"package a\n\nvar X = 1": "3 行",
		"":                       "0 行",
	}
	for src, want := range cases {
		if page := string(renderSource("a.go", []byte(src), "/", nil)); !strings.Contains(page, want) {
			t.Errorf("renderSource(%q) does not say %q", src, want)
		}
	}
}

func TestRenderSourceSurvivesBadInput(t *testing.T) {
	// Invalid UTF-8 and unlexable Go must still render the bytes.
	src := []byte("package a\ntype \xff\n")
	page := string(renderSource("a.go", src, "/", nil))
	if !strings.Contains(page, `<span class="ln" id="L1">`) {
		t.Fatal("no lines rendered")
	}
	if stripTags(page) == "" {
		t.Fatal("content vanished")
	}
}

func TestBackRoute(t *testing.T) {
	cases := map[string]string{
		"":             "/",
		"#/":           "/#/",
		"#/type/a%23b": "/#/type/a%23b",
		"javascript:x": "/",
		"//evil.test":  "/",
		"#/a\"b":       "/",
		"#/a b":        "/",
	}
	for from, want := range cases {
		if got := backRoute(from); got != want {
			t.Errorf("backRoute(%q) = %q, want %q", from, got, want)
		}
	}
}

func TestRenderSourceBackLink(t *testing.T) {
	page := string(renderSource("a.go", []byte("package a\n"), "/#/type/example.com%2Fapp", nil))
	if !strings.Contains(page, `<a class="back" href="/#/type/example.com%2Fapp">← 返回</a>`) {
		t.Fatalf("back link missing:\n%s", page[:400])
	}
}
