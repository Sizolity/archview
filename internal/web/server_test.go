package web

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Sizolity/archview/internal/model"
)

// stripTags removes markup so a highlighted page can be compared with the
// plain text it came from.
func stripTags(s string) string {
	return html.UnescapeString(regexp.MustCompile(`<[^>]*>`).ReplaceAllString(s, ""))
}

func TestServerRoutes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n\n// A is a type.\ntype A struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		Root: root,
		UI:   []byte("<!doctype html><html>ui<script src=\"/ui.js\"></script></html>"),
		JS:   []byte("window.ArchView = {esc: function () {}};"),
		Load: func(context.Context) (*model.Module, error) {
			return &model.Module{
				Path:     "example.com/mod",
				Root:     root,
				Packages: []*model.Package{{Path: "example.com/mod/a", Name: "a"}},
				Warnings: []string{"类型检查已跳过"},
			}, nil
		},
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	get := func(path string) (int, string) {
		t.Helper()
		resp, err := ts.Client().Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, string(b)
	}

	if code, body := get("/"); code != http.StatusOK || !strings.Contains(body, "ui") {
		t.Fatalf("GET / = %d %q", code, body)
	}
	if code, body := get("/ui.js"); code != http.StatusOK || !strings.Contains(body, "ArchView") {
		t.Fatalf("GET /ui.js = %d %q", code, body)
	}
	if code, body := get("/healthz"); code != http.StatusOK || !strings.Contains(body, "ok") {
		t.Fatalf("GET /healthz = %d %q", code, body)
	}

	code, body := get("/model.json")
	if code != http.StatusOK {
		t.Fatalf("GET /model.json = %d %q", code, body)
	}
	var m model.Module
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatal(err)
	}
	if m.Path != "example.com/mod" || len(m.Packages) != 1 {
		t.Fatalf("model = %+v", m)
	}
	// A degraded type check must reach the browser, not just the log.
	if len(m.Warnings) != 1 || m.Warnings[0] != "类型检查已跳过" {
		t.Fatalf("warnings = %v", m.Warnings)
	}

	code, body = get("/source?path=a.go")
	if code != http.StatusOK || !strings.Contains(stripTags(body), "package a") || !strings.Contains(body, `id="L1"`) {
		t.Fatalf("GET /source = %d %q", code, body)
	}
	// It renders as a fenced code block: a framed card with a title bar, a
	// language tag and a line count, and one gutter number per line.
	if !strings.Contains(body, `class="block"`) || !strings.Contains(body, `class="lang">go`) {
		t.Fatalf("source page is not a code block: %q", body)
	}
	// The file has four lines; a trailing newline is not a fifth.
	if !strings.Contains(body, "4 行") {
		t.Fatalf("line count missing or wrong: %q", body)
	}
	// Go is highlighted, so keywords and comments stand out from the text.
	for _, want := range []string{`class="kw">package`, `class="kw">struct`, `class="com">// A is a type.`} {
		if !strings.Contains(body, want) {
			t.Fatalf("highlighting missing %q: %q", want, body)
		}
	}

	for _, path := range []string{"/source", "/source?path=../secret.go", "/source?path=README.md"} {
		if code, _ := get(path); code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", path, code)
		}
	}
}

// sourceServer starts a server rooted at dir with a stub loader, so the
// source endpoint can be tested without a real module scan.
func sourceServer(t *testing.T, dir string) *httptest.Server {
	t.Helper()
	s := &Server{
		Root: dir,
		UI:   []byte("<!doctype html><html>ui</html>"),
		JS:   []byte("window.ArchView = {};"),
		Load: func(context.Context) (*model.Module, error) {
			return &model.Module{Path: "example.com/mod", Root: dir}, nil
		},
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func fetchText(t *testing.T, ts *httptest.Server, path string) (int, string) {
	t.Helper()
	resp, err := ts.Client().Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

func TestSourceRejectsOversizeFile(t *testing.T) {
	dir := t.TempDir()
	big := bytes.Repeat([]byte("a"), 2<<20+1)
	if err := os.WriteFile(filepath.Join(dir, "big.go"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	ts := sourceServer(t, dir)

	code, body := fetchText(t, ts, "/source?path=big.go")
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("GET /source big.go = %d, want 413 (%q)", code, body)
	}
}

func TestSourceEscapesInvalidUTF8(t *testing.T) {
	dir := t.TempDir()
	data := []byte("package a\n// \xff\xfe <b>raw</b>\n")
	if err := os.WriteFile(filepath.Join(dir, "bad.go"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	ts := sourceServer(t, dir)

	code, body := fetchText(t, ts, "/source?path=bad.go")
	if code != http.StatusOK {
		t.Fatalf("GET /source bad.go = %d, want 200", code)
	}
	if strings.Contains(body, "<b>raw</b>") {
		t.Fatalf("source markup leaked unescaped: %q", body)
	}
	if !strings.Contains(body, "&lt;b&gt;raw&lt;/b&gt;") {
		t.Fatalf("expected escaped markup, got %q", body)
	}
}
