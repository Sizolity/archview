package cache

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sizolity/archview/internal/model"
)

func TestWatchLoopNotifiesOncePerChange(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, filepath.Join(dir, "a.go"), "package a\n")

	c := New(dir, func(context.Context, string) (*model.Module, error) {
		return &model.Module{Root: dir}, nil
	})
	w := NewWatcher()
	ch := w.Subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go WatchLoop(ctx, c, w, 20*time.Millisecond)
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

func TestWatcherNotifyIsNonBlocking(t *testing.T) {
	w := NewWatcher()
	ch := w.Subscribe() // buffered with room for one
	w.Notify()
	w.Notify() // a subscriber that does not read must not stall the loop
	w.Unsubscribe(ch)
	w.Notify()
}
