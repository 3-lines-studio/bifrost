package data

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func changes(t *testing.T, cache *Cache[string, string]) <-chan string {
	t.Helper()
	seen := make(chan string, 16)
	cache.OnChange(func(key string) { seen <- key })
	return seen
}

func wait(t *testing.T, seen <-chan string) string {
	t.Helper()
	select {
	case key := <-seen:
		return key
	case <-time.After(5 * time.Second):
		t.Fatal("no change arrived")
		return ""
	}
}

func ready[V any](t *testing.T, cache *Cache[string, V], key string) Entry[V] {
	t.Helper()
	entry := cache.Get(key)
	if entry.State != Ready {
		t.Fatalf("state %v, error %v", entry.State, entry.Err)
	}
	return entry
}

func TestGetLoadsOnce(t *testing.T) {
	var loads atomic.Int32
	release := make(chan struct{})
	cache := New(func(ctx context.Context, key string) (string, error) {
		loads.Add(1)
		<-release
		return key + "!", nil
	})
	defer cache.Close()
	seen := changes(t, cache)

	if entry := cache.Get("a"); entry.State != Loading {
		t.Fatalf("first state %v", entry.State)
	}
	var waitGroup sync.WaitGroup
	for range 10 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			cache.Get("a")
		}()
	}
	waitGroup.Wait()
	if got := loads.Load(); got != 1 {
		t.Fatalf("loads %d", got)
	}
	close(release)
	if key := wait(t, seen); key != "a" {
		t.Fatalf("key %q", key)
	}
	entry := ready(t, cache, "a")
	if entry.Value != "a!" {
		t.Fatalf("value %q", entry.Value)
	}
	if entry.Loaded.IsZero() {
		t.Fatal("no load time")
	}
	if got := loads.Load(); got != 1 {
		t.Fatalf("loads %d", got)
	}
}

func TestInvalidateAndRefresh(t *testing.T) {
	var loads atomic.Int32
	cache := New(func(ctx context.Context, key string) (string, error) {
		n := loads.Add(1)
		return string(rune('0' + n)), nil
	})
	defer cache.Close()
	seen := changes(t, cache)

	cache.Get("a")
	wait(t, seen)
	if entry := ready(t, cache, "a"); entry.Value != "1" {
		t.Fatalf("value %q", entry.Value)
	}
	cache.Invalidate("a")
	cache.Get("a")
	wait(t, seen)
	if entry := ready(t, cache, "a"); entry.Value != "2" {
		t.Fatalf("value %q", entry.Value)
	}
	cache.Refresh("a")
	wait(t, seen)
	if entry := ready(t, cache, "a"); entry.Value != "3" {
		t.Fatalf("value %q", entry.Value)
	}
}

func TestFailureStaysUntilRefresh(t *testing.T) {
	var loads atomic.Int32
	failing := true
	cache := New(func(ctx context.Context, key string) (string, error) {
		loads.Add(1)
		if failing {
			return "", errors.New("down")
		}
		return "up", nil
	})
	defer cache.Close()
	seen := changes(t, cache)

	cache.Get("a")
	wait(t, seen)
	entry := cache.Get("a")
	if entry.State != Failed || entry.Err == nil {
		t.Fatalf("entry %+v", entry)
	}
	if got := loads.Load(); got != 1 {
		t.Fatalf("a failed value was loaded again: %d", got)
	}
	failing = false
	cache.Refresh("a")
	wait(t, seen)
	if entry := ready(t, cache, "a"); entry.Value != "up" {
		t.Fatalf("value %q", entry.Value)
	}
}

func TestSetAndUpdate(t *testing.T) {
	cache := New(func(ctx context.Context, key string) (string, error) {
		return "loaded", nil
	})
	defer cache.Close()
	seen := changes(t, cache)

	cache.Set("a", "written")
	if key := wait(t, seen); key != "a" {
		t.Fatalf("key %q", key)
	}
	if entry := ready(t, cache, "a"); entry.Value != "written" {
		t.Fatalf("value %q", entry.Value)
	}
	cache.Update("a", func(value string) string { return value + "!" })
	wait(t, seen)
	if entry := ready(t, cache, "a"); entry.Value != "written!" {
		t.Fatalf("value %q", entry.Value)
	}
	cache.Update("missing", func(value string) string { return "no" })
	select {
	case key := <-seen:
		t.Fatalf("update of a missing key notified %q", key)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCloseDropsTheAnswer(t *testing.T) {
	release := make(chan struct{})
	cache := New(func(ctx context.Context, key string) (string, error) {
		<-release
		return "late", nil
	})
	var notified atomic.Int32
	cache.OnChange(func(string) { notified.Add(1) })
	cache.Get("a")
	cache.Close()
	close(release)
	time.Sleep(50 * time.Millisecond)
	if notified.Load() != 0 {
		t.Fatal("a closed cache notified a change")
	}
	if entry := cache.Get("a"); entry.State != Loading {
		t.Fatalf("state %v", entry.State)
	}
}

func TestForgetDuringLoad(t *testing.T) {
	release := make(chan struct{})
	cache := New(func(ctx context.Context, key string) (string, error) {
		<-release
		return "late", nil
	})
	var notified atomic.Int32
	cache.OnChange(func(string) { notified.Add(1) })
	cache.Get("a")
	cache.Forget("a")
	close(release)
	time.Sleep(50 * time.Millisecond)
	if notified.Load() != 0 {
		t.Fatal("a forgotten key notified a change")
	}
	if entry := cache.Get("a"); entry.State != Loading {
		t.Fatalf("state %v", entry.State)
	}
}

func TestUnsubscribe(t *testing.T) {
	cache := New(func(ctx context.Context, key string) (string, error) { return "value", nil })
	defer cache.Close()
	var calls atomic.Int32
	off := cache.OnChange(func(string) { calls.Add(1) })
	off()
	off()
	cache.Set("a", "value")
	if calls.Load() != 0 {
		t.Fatalf("a removed listener was called %d times", calls.Load())
	}
}
