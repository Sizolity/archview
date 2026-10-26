package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Server serves the explorer UI, the model snapshot and a read-only source view.
type Server struct {
	Root string
	UI   []byte
	Load func(context.Context) (*Module, error)
}

// Handler returns the read-only HTTP surface.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /model.json", s.model)
	mux.HandleFunc("GET /source", s.source)
	mux.HandleFunc("GET /healthz", s.health)
	return mux
}

func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.UI)
}

func (s *Server) model(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	m, err := s.Load(ctx)
	if err != nil {
		http.Error(w, "扫描失败: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(m); err != nil {
		http.Error(w, "编码失败: "+err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintln(w, "ok")
}

func (s *Server) source(w http.ResponseWriter, r *http.Request) {
	requested := r.URL.Query().Get("path")
	if requested == "" {
		http.Error(w, "缺少 path", http.StatusBadRequest)
		return
	}
	clean := filepath.Clean(filepath.FromSlash(requested))
	if filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, "..") {
		http.Error(w, "非法路径", http.StatusBadRequest)
		return
	}
	if !strings.HasSuffix(clean, ".go") {
		http.Error(w, "只允许查看 .go 文件", http.StatusBadRequest)
		return
	}
	full := filepath.Join(s.Root, clean)
	rel, err := filepath.Rel(s.Root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		http.Error(w, "路径越界", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.Error(w, "文件不存在", http.StatusNotFound)
		return
	}
	if info.Size() > 2<<20 {
		http.Error(w, "文件过大", http.StatusRequestEntityTooLarge)
		return
	}
	data, err := os.ReadFile(full)
	if err != nil {
		http.Error(w, "读取失败", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(renderSource(filepath.ToSlash(clean), data))
}

func renderSource(path string, data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	b.WriteString("<title>" + html.EscapeString(path) + "</title><style>")
	b.WriteString(`body{margin:0;background:#0f172a;color:#e2e8f0;font:13px/1.55 ui-monospace,SFMono-Regular,Consolas,monospace}
header{position:sticky;top:0;display:flex;gap:16px;align-items:center;padding:10px 18px;background:#111827;border-bottom:1px solid #1f2937;font-family:system-ui,sans-serif}
header a{color:#93c5fd;text-decoration:none}header a:hover{text-decoration:underline}
header .path{color:#cbd5e1;font-family:ui-monospace,Consolas,monospace}
pre{margin:0;padding:14px 0 60px;overflow:auto}
.ln{display:block;padding:0 18px;white-space:pre}
.ln:target{background:#1e3a8a;box-shadow:inset 3px 0 0 #60a5fa}
.no{display:inline-block;width:5ch;color:#64748b;text-decoration:none;user-select:none}
.ln:target .no{color:#bfdbfe}
`)
	b.WriteString("</style></head><body><header><a href=\"/\" onclick=\"history.back();return false\">返回</a>")
	b.WriteString("<span class=\"path\">" + html.EscapeString(path) + "</span></header><pre>")
	for i, line := range lines {
		n := i + 1
		fmt.Fprintf(&b, "<span class=\"ln\" id=\"L%d\"><a class=\"no\" href=\"#L%d\">%d</a>%s</span>\n",
			n, n, n, html.EscapeString(line))
	}
	b.WriteString("</pre></body></html>")
	return []byte(b.String())
}
