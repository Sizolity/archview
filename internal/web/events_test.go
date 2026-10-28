package web

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Sizolity/archview/internal/cache"
	"github.com/Sizolity/archview/internal/model"
)

func watcherServer(t *testing.T, dir string) (*Server, *httptest.Server) {
	t.Helper()
	s := &Server{
		Root:  dir,
		Watch: cache.NewWatcher(),
		UI:    []byte("ui"),
		JS:    []byte("js"),
		Load: func(context.Context) (*model.Module, error) {
			return &model.Module{Root: dir}, nil
		},
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func TestEventsStreamsReload(t *testing.T) {
	dir := t.TempDir()
	s, ts := watcherServer(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type = %q", ct)
	}

	br := bufio.NewReader(resp.Body)
	line, err := br.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, ": connected") {
		t.Fatalf("first line = %q, err = %v", line, err)
	}

	s.Watch.Notify()
	for range 6 {
		line, err = br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading events: %v", err)
		}
		if strings.HasPrefix(line, "event: reload") {
			return
		}
	}
	t.Fatal("no reload event arrived")
}

func TestEventsWithoutWatchIsNotFound(t *testing.T) {
	dir := t.TempDir()
	s := &Server{
		Root: dir,
		UI:   []byte("ui"),
		JS:   []byte("js"),
		Load: func(context.Context) (*model.Module, error) { return &model.Module{Root: dir}, nil },
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /events = %d, want 404 when -watch is off", resp.StatusCode)
	}
}
