package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Server serves the explorer UI, the model snapshot and a read-only source view.
// Load may return a shared, read-only model (the cache hands out one pointer),
// so handlers must not mutate it.
type Server struct {
	Root  string
	Watch *watcher
	UI    []byte
	JS    []byte
	Load  func(context.Context) (*Module, error)
}

// Handler returns the read-only HTTP surface.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /ui.js", s.script)
	mux.HandleFunc("GET /model.json", s.model)
	mux.HandleFunc("GET /source", s.source)
	mux.HandleFunc("GET /events", s.events)
	mux.HandleFunc("GET /healthz", s.health)
	return mux
}

func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.UI)
}

func (s *Server) script(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.JS)
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

// events streams a "reload" event whenever the watched tree moves. It is only
// served when -watch is on, and it never touches the model itself.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if s.Watch == nil {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "不支持流式响应", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ch := s.Watch.subscribe()
	defer s.Watch.unsubscribe(ch)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			if _, err := fmt.Fprint(w, "event: reload\ndata: {}\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) source(w http.ResponseWriter, r *http.Request) {
	if s.Root == "" {
		// An imported IR has no module behind it.
		http.Error(w, "导入的模型没有源码目录", http.StatusNotFound)
		return
	}
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
