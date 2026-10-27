package main

import (
	"context"
	"sync"
	"time"
)

// watcher polls the module fingerprint and fans a "the tree moved" event out to
// every connected page. Polling keeps the tool standard-library only; a scan is
// seconds, so a 500ms poll is not the bottleneck.
type watcher struct {
	mu   sync.Mutex
	subs map[chan struct{}]bool
}

func newWatcher() *watcher {
	return &watcher{subs: map[chan struct{}]bool{}}
}

// subscribe returns a channel that receives one value per change. The caller
// must unsubscribe.
func (w *watcher) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	w.mu.Lock()
	w.subs[ch] = true
	w.mu.Unlock()
	return ch
}

func (w *watcher) unsubscribe(ch chan struct{}) {
	w.mu.Lock()
	delete(w.subs, ch)
	w.mu.Unlock()
}

// notify tells every subscriber the tree moved, without blocking on a slow one.
func (w *watcher) notify() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for ch := range w.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// watchLoop polls the fingerprint and notifies on every change. It never
// rescans: the reload it triggers goes through the cache, which does that.
func watchLoop(ctx context.Context, c *modelCache, w *watcher, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	last, err := c.fingerprint()
	if err != nil {
		last = ""
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sum, err := c.fingerprint()
			if err != nil || sum == last {
				continue
			}
			last = sum
			w.notify()
		}
	}
}
