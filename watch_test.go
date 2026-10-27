package main

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func watcherServer(t *testing.T, dir string) (*Server, *httptest.Server) {
	t.Helper()
	s := &Server{
		Root:  dir,
		Watch: newWatcher(),
		UI:    []byte("ui"),
		JS:    []byte("js"),
		Load: func(context.Context) (*Module, error) {
			return &Module{Root: dir}, nil
		},
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func TestWatchLoopNotifiesOncePerChange(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, filepath.Join(dir, "a.go"), "package a\n")

	c := newModelCache(dir, func(context.Context, string) (*Module, error) {
		return &Module{Root: dir}, nil
	})
	w := newWatcher()
	ch := w.subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watchLoop(ctx, c, w, 20*time.Millisecond)
	time.Sleep(100 * time.Millisecond) // let the loop read its first fingerprint

	writeSource(t, filepath.Join(dir, "a.go"), "package a\n\nvar X = 1\n")
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("no notification after a source change")
	}

	// Give the loop a few more ticks: it must not keep notifying on the same
	// tree. Windows can bump a file's timestamp once more when the handle
	// closes, so allow one extra signal instead of demanding silence.
	extra := 0
	deadline := time.After(400 * time.Millisecond)
loop:
	for {
		select {
		case <-ch:
			extra++
			if extra > 1 {
				t.Fatalf("notified %d extra times without another change", extra)
			}
		case <-deadline:
			break loop
		}
	}
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

	s.Watch.notify()
	for i := 0; i < 6; i++ {
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
		Load: func(context.Context) (*Module, error) { return &Module{Root: dir}, nil },
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

func TestWatcherNotifyIsNonBlocking(t *testing.T) {
	w := newWatcher()
	ch := w.subscribe() // buffered with room for one
	w.notify()
	w.notify() // a subscriber that does not read must not stall the loop
	w.unsubscribe(ch)
	w.notify()
}
