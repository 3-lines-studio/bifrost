package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileWatcher_StartClosesChannels(t *testing.T) {
	dir := t.TempDir()
	w := newFileWatcher(dir)

	ctx, cancel := context.WithCancel(context.Background())
	goChanges, feChanges := w.Start(ctx)

	cancel()

	select {
	case _, ok := <-goChanges:
		if ok {
			t.Fatal("goChanges should be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("goChanges was not closed after context cancellation")
	}

	select {
	case _, ok := <-feChanges:
		if ok {
			t.Fatal("feChanges should be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("feChanges was not closed after context cancellation")
	}
}

func TestFileWatcher_DetectsGoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	w := newFileWatcher(dir)
	ctx := t.Context()

	goChanges, feChanges := w.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	select {
	case <-goChanges:
	case <-time.After(2 * time.Second):
		t.Fatal("expected goChanges signal")
	}

	select {
	case <-feChanges:
		t.Fatal("did not expect feChanges signal")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestFileWatcher_DetectsFrontendChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "home.tsx")
	if err := os.WriteFile(path, []byte("export default function Home() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	w := newFileWatcher(dir)
	ctx := t.Context()

	goChanges, feChanges := w.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(path, []byte("export default function Home() { return null }\n"), 0644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	select {
	case <-feChanges:
	case <-time.After(2 * time.Second):
		t.Fatal("expected feChanges signal")
	}

	select {
	case <-goChanges:
		t.Fatal("did not expect goChanges signal")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestFileWatcher_ExcludesNodeModules(t *testing.T) {
	dir := t.TempDir()
	nodeModules := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(nodeModules, 0755); err != nil {
		t.Fatalf("failed to create node_modules: %v", err)
	}
	path := filepath.Join(nodeModules, "foo.go")
	if err := os.WriteFile(path, []byte("package foo\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	w := newFileWatcher(dir)
	ctx := t.Context()

	goChanges, _ := w.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(path, []byte("package bar\n"), 0644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	select {
	case <-goChanges:
		t.Fatal("node_modules should be excluded")
	case <-time.After(500 * time.Millisecond):
	}
}

func TestFileWatcher_DebounceCoalescesRapidChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.tsx")
	if err := os.WriteFile(path, []byte("export default function App() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	w := newFileWatcher(dir)
	ctx := t.Context()

	_, feChanges := w.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	// Fire many writes in quick succession.
	for i := range 10 {
		if err := os.WriteFile(path, fmt.Appendf(nil, "const x = %d\n", i), 0644); err != nil {
			t.Fatalf("failed to update file: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// We should get at least one signal, but rapid events within a debounce
	// window should not produce one signal per write.
	signals := 0
	done := time.After(800 * time.Millisecond)
loop:
	for {
		select {
		case <-feChanges:
			signals++
			if signals > 3 {
				t.Fatalf("expected debounced signals, got %d", signals)
			}
		case <-done:
			break loop
		}
	}
	if signals == 0 {
		t.Fatal("expected at least one frontend change signal")
	}
}

func TestFileWatcher_RenameEventDetected(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.go")
	newPath := filepath.Join(dir, "new.go")
	if err := os.WriteFile(oldPath, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	w := newFileWatcher(dir)
	ctx := t.Context()

	goChanges, _ := w.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("failed to rename file: %v", err)
	}

	select {
	case <-goChanges:
	case <-time.After(2 * time.Second):
		t.Fatal("expected goChanges signal after rename")
	}
}

func TestFileWatcher_NewDirectoryWatched(t *testing.T) {
	dir := t.TempDir()
	w := newFileWatcher(dir)
	ctx := t.Context()

	goChanges, _ := w.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	subDir := filepath.Join(dir, "pages")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	path := filepath.Join(subDir, "home.go")
	if err := os.WriteFile(path, []byte("package pages\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	select {
	case <-goChanges:
	case <-time.After(2 * time.Second):
		t.Fatal("expected goChanges for new directory")
	}
}
