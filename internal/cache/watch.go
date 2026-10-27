package cache

import (
	"context"
	"sync"
	"time"
)

// Watcher polls the module fingerprint and fans a "the tree moved" event out to
// every connected page. Polling keeps the tool standard-library only; a scan is
// seconds, so a 500ms poll is not the bottleneck.
type Watcher struct {
	mu   sync.Mutex
	subs map[chan struct{}]bool
}

func NewWatcher() *Watcher {
	return &Watcher{subs: map[chan struct{}]bool{}}
}

// subscribe returns a channel that receives one value per change. The caller
// must unsubscribe.
func (w *Watcher) Subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	w.mu.Lock()
	w.subs[ch] = true
	w.mu.Unlock()
	return ch
}

func (w *Watcher) Unsubscribe(ch chan struct{}) {
	w.mu.Lock()
	delete(w.subs, ch)
	w.mu.Unlock()
}

// notify tells every subscriber the tree moved, without blocking on a slow one.
func (w *Watcher) Notify() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for ch := range w.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// WatchLoop polls the fingerprint and notifies on every change. It never
// rescans: the reload it triggers goes through the cache, which does that.
func WatchLoop(ctx context.Context, c *Cache, w *Watcher, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	last, err := c.Fingerprint()
	if err != nil {
		last = ""
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sum, err := c.Fingerprint()
			if err != nil || sum == last {
				continue
			}
			last = sum
			w.Notify()
		}
	}
}
