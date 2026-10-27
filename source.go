package main

import (
	_ "embed"
	"fmt"
	"go/scanner"
	"go/token"
	"html"
	"strings"
)

//go:embed source.css
var sourceCSS []byte

// span is one highlighted token: a byte range of the file and the CSS class
// that colours it.
type span struct {
	start, end int
	class      string
}

// renderSource renders a file the way a fenced code block looks in Markdown: a
// light card with a title bar, a scrollable monospace body, a line-number
// gutter that stays out of the copied text, and Go highlighting so a long
// string literal reads as a string instead of as stray text.
func renderSource(path string, data []byte) []byte {
	lines := lineRanges(data)
	spans := goSpans(data)

	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	b.WriteString("<title>" + html.EscapeString(path) + "</title><style>")
	b.WriteString(string(sourceCSS))
	b.WriteString("</style></head><body><main class=\"page\">")
	b.WriteString("<nav class=\"bar\"><a class=\"back\" href=\"/\" onclick=\"history.back();return false\">← 返回</a>")
	b.WriteString("<span class=\"path\">" + html.EscapeString(path) + "</span></nav>")
	b.WriteString("<section class=\"block\"><div class=\"head\"><span class=\"file\">" + html.EscapeString(path) + "</span>")
	b.WriteString("<span class=\"lang\">go</span><span class=\"count\">" + fmt.Sprint(len(lines)) + " 行</span></div>")
	b.WriteString("<pre class=\"code\">")

	si := 0
	for i, lr := range lines {
		n := i + 1
		// Skip spans that ended before this line; spans still crossing keep si.
		for si < len(spans) && spans[si].end <= lr.start {
			si++
		}
		fmt.Fprintf(&b, "<span class=\"ln\" id=\"L%d\"><a class=\"no\" href=\"#L%d\">%d</a>", n, n, n)
		writeLine(&b, data, lr.start, lr.end, spans[si:])
		b.WriteString("</span>")
	}
	b.WriteString("</pre></section></main></body></html>")
	return []byte(b.String())
}

type lineRange struct{ start, end int }

// lineRanges splits the file into lines by byte offset. A trailing newline does
// not open a new line, and an empty file has none.
func lineRanges(data []byte) []lineRange {
	if len(data) == 0 {
		return nil
	}
	var out []lineRange
	start := 0
	for i, c := range data {
		if c != '\n' {
			continue
		}
		end := i
		if end > start && data[end-1] == '\r' {
			end--
		}
		out = append(out, lineRange{start, end})
		start = i + 1
	}
	if start < len(data) {
		out = append(out, lineRange{start, len(data)})
	}
	return out
}

// writeLine emits one line, wrapping the parts that belong to a token.
func writeLine(b *strings.Builder, data []byte, start, end int, spans []span) {
	cursor := start
	for _, s := range spans {
		if s.start >= end {
			break
		}
		from := s.start
		if from < start {
			from = start
		}
		to := s.end
		if to > end {
			to = end
		}
		if to <= from {
			continue
		}
		if from > cursor {
			b.WriteString(html.EscapeString(string(data[cursor:from])))
		}
		b.WriteString(`<span class="` + s.class + `">`)
		b.WriteString(html.EscapeString(string(data[from:to])))
		b.WriteString("</span>")
		cursor = to
	}
	if cursor < end {
		b.WriteString(html.EscapeString(string(data[cursor:end])))
	}
}

// goSpans tokenizes the file with go/scanner and returns the ranges worth
// colouring. Unlexable input is not fatal: the scanner simply yields fewer
// spans and the text still renders.
func goSpans(data []byte) []span {
	if len(data) == 0 {
		return nil
	}
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(data))
	var s scanner.Scanner
	s.Init(file, data, func(_ token.Position, _ string) {}, scanner.ScanComments)

	var out []span
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		class := tokenClass(tok)
		if class == "" {
			continue
		}
		text := lit
		if text == "" {
			// Keywords and operators carry no literal; the token knows its own
			// spelling and that is what sits in the source.
			text = tok.String()
		}
		if text == "" || text == "\n" {
			continue
		}
		start := file.Offset(pos)
		if start < 0 || start+len(text) > len(data) {
			continue
		}
		out = append(out, span{start: start, end: start + len(text), class: class})
	}
	return out
}

func tokenClass(tok token.Token) string {
	switch {
	case tok == token.COMMENT:
		return "com"
	case tok == token.STRING || tok == token.CHAR:
		return "str"
	case tok == token.INT || tok == token.FLOAT || tok == token.IMAG:
		return "num"
	case tok.IsKeyword():
		return "kw"
	}
	return ""
}
