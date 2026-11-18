// Package data holds what a screen reads from the server, so the screen shows
// it at once, loads it once, and loads it again only when something changed.
//
// A Cache is read while the view is built, on the goroutine that draws, and
// loads on a goroutine of its own, so neither waits for the other:
//
//	notes := data.New(func(ctx context.Context, _ struct{}) ([]Note, error) {
//		return client.GetJSON[[]Note](ctx, "/api/notes", session.Header())
//	})
//
//	func (a *app) view(c *ui.Context) {
//		ui.Column(c).Children(func() {
//			switch entry := notes.Get(struct{}{}); entry.State {
//			case data.Loading:
//				ui.Spinner(c)
//			case data.Failed:
//				ui.Text(c, entry.Err.Error())
//			default:
//				a.list(c, entry.Value)
//			}
//		})
//	}
//
// A screen reads a value that changed from OnChange, which runs on the
// goroutine that loaded it: apply it in MyGo's Window.Update.
package data

import (
	"context"
	"sync"
	"time"
)

// State is what a Cache knows of a key.
type State uint8

const (
	// Loading is a value on its way: the cache asks for it.
	Loading State = iota + 1
	// Ready is a value it holds.
	Ready
	// Failed is the error its last load answered.
	Failed
)

// Entry is what a Cache holds of one key.
type Entry[V any] struct {
	Value  V
	Err    error
	State  State
	Loaded time.Time
}

// Loader returns the value of a key.
type Loader[K comparable, V any] func(context.Context, K) (V, error)

// Cache holds values by key, loads them on demand, and tells what changed.
// It is safe for concurrent use.
type Cache[K comparable, V any] struct {
	load  Loader[K, V]
	ctx   context.Context
	stop  context.CancelFunc
	mu    sync.Mutex
	known map[K]*entry[V]
	seen  []func(K)
}

type entry[V any] struct {
	value  V
	err    error
	state  State
	loaded time.Time
	// stale is a value the cache holds but must load again.
	stale bool
	// loading is set while a load is on its way.
	loading bool
}

// New returns an empty cache that loads its keys with load.
func New[K comparable, V any](load Loader[K, V]) *Cache[K, V] {
	ctx, stop := context.WithCancel(context.Background())
	return &Cache[K, V]{load: load, ctx: ctx, stop: stop, known: map[K]*entry[V]{}}
}

// Get returns what the cache holds of key, and asks for it when it holds
// nothing, or holds a value Invalidate marked stale. It returns at once: the
// value arrives through the cache, and OnChange says when.
func (c *Cache[K, V]) Get(key K) Entry[V] {
	c.mu.Lock()
	known := c.known[key]
	if known == nil {
		known = &entry[V]{state: Loading}
		c.known[key] = known
	}
	if !known.loading && (known.state == Loading || known.stale) {
		c.start(key, known)
	}
	view := known.view()
	c.mu.Unlock()
	return view
}

// Refresh loads key now, whatever the cache holds of it.
func (c *Cache[K, V]) Refresh(key K) {
	c.mu.Lock()
	known := c.known[key]
	if known == nil {
		known = &entry[V]{state: Loading}
		c.known[key] = known
	}
	known.stale = true
	if !known.loading {
		c.start(key, known)
	}
	c.mu.Unlock()
}

// Set gives key a value the app knows without asking, as one it just wrote.
func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	known := c.known[key]
	if known == nil {
		known = &entry[V]{}
		c.known[key] = known
	}
	known.value, known.err, known.state = value, nil, Ready
	known.loaded, known.stale = time.Now(), false
	c.mu.Unlock()
	c.changed(key)
}

// Update changes the value of key with change, as a row added to a list the
// cache holds. It does nothing when the cache holds no value of key.
func (c *Cache[K, V]) Update(key K, change func(V) V) {
	c.mu.Lock()
	known := c.known[key]
	if known == nil || known.state != Ready {
		c.mu.Unlock()
		return
	}
	known.value = change(known.value)
	c.mu.Unlock()
	c.changed(key)
}

// Invalidate makes the cache load the keys again the next time a screen asks
// for them.
func (c *Cache[K, V]) Invalidate(keys ...K) {
	c.mu.Lock()
	for _, key := range keys {
		if known := c.known[key]; known != nil {
			known.stale = true
		}
	}
	c.mu.Unlock()
}

// Forget drops what the cache holds of the keys, as the ones of a screen that
// closed.
func (c *Cache[K, V]) Forget(keys ...K) {
	c.mu.Lock()
	for _, key := range keys {
		delete(c.known, key)
	}
	c.mu.Unlock()
}

// OnChange adds fn to the ones called when a value of the cache changes, with
// its key, and returns the function that removes it. fn runs on the goroutine
// that loaded the value, or on the one that set it.
func (c *Cache[K, V]) OnChange(fn func(K)) (off func()) {
	if fn == nil {
		return func() {}
	}
	c.mu.Lock()
	c.seen = append(c.seen, fn)
	index := len(c.seen) - 1
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if index < len(c.seen) && c.seen[index] != nil {
			c.seen[index] = nil
		}
	}
}

// Close stops the loads on their way: what they answer is dropped.
func (c *Cache[K, V]) Close() { c.stop() }

// start loads key on a goroutine of its own. The caller holds the lock.
func (c *Cache[K, V]) start(key K, known *entry[V]) {
	known.loading = true
	go func() {
		value, err := c.load(c.ctx, key)
		c.mu.Lock()
		if c.ctx.Err() != nil || c.known[key] != known {
			c.mu.Unlock()
			return
		}
		known.loading = false
		known.loaded = time.Now()
		if err != nil {
			known.err, known.state, known.stale = err, Failed, false
		} else {
			known.value, known.err, known.state, known.stale = value, nil, Ready, false
		}
		c.mu.Unlock()
		c.changed(key)
	}()
}

func (c *Cache[K, V]) changed(key K) {
	c.mu.Lock()
	listeners := make([]func(K), len(c.seen))
	copy(listeners, c.seen)
	c.mu.Unlock()
	for _, listener := range listeners {
		if listener != nil {
			listener(key)
		}
	}
}

// view returns the value of the entry as the screen reads it.
func (e *entry[V]) view() Entry[V] {
	return Entry[V]{Value: e.value, Err: e.err, State: e.state, Loaded: e.loaded}
}
