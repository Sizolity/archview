package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerRoutes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n\n// A is a type.\ntype A struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		Root: root,
		UI:   []byte("<!doctype html><html>ui</html>"),
		Load: func(context.Context) (*Module, error) {
			return &Module{
				Path:     "example.com/mod",
				Root:     root,
				Packages: []*Package{{Path: "example.com/mod/a", Name: "a"}},
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
	if code, body := get("/healthz"); code != http.StatusOK || !strings.Contains(body, "ok") {
		t.Fatalf("GET /healthz = %d %q", code, body)
	}

	code, body := get("/model.json")
	if code != http.StatusOK {
		t.Fatalf("GET /model.json = %d %q", code, body)
	}
	var m Module
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatal(err)
	}
	if m.Path != "example.com/mod" || len(m.Packages) != 1 {
		t.Fatalf("model = %+v", m)
	}

	code, body = get("/source?path=a.go")
	if code != http.StatusOK || !strings.Contains(body, "package a") || !strings.Contains(body, `id="L1"`) {
		t.Fatalf("GET /source = %d %q", code, body)
	}

	for _, path := range []string{"/source", "/source?path=../secret.go", "/source?path=README.md"} {
		if code, _ := get(path); code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", path, code)
		}
	}
}
