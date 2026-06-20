package main

import (
	"context"
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

func TestFileWatcher_ScanDetectsGoChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	w := newFileWatcher(dir)
	var goPending, fePending bool

	w.scan(true, &goPending, &fePending, nil, nil)
	if goPending || fePending {
		t.Fatal("first scan should not report pending changes")
	}

	// Ensure mtime changes.
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	goPending, fePending = false, false
	w.scan(false, &goPending, &fePending, nil, nil)
	if !goPending {
		t.Fatal("expected .go file change to be detected")
	}
	if fePending {
		t.Fatal("did not expect frontend change for .go file")
	}
}

func TestFileWatcher_ScanDetectsFrontendChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "home.tsx")
	if err := os.WriteFile(path, []byte("export default function Home() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	w := newFileWatcher(dir)
	var goPending, fePending bool
	w.scan(true, &goPending, &fePending, nil, nil)

	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(path, []byte("export default function Home() { return <div /> }\n"), 0644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	goPending, fePending = false, false
	w.scan(false, &goPending, &fePending, nil, nil)
	if goPending {
		t.Fatal("did not expect go change for .tsx file")
	}
	if !fePending {
		t.Fatal("expected .tsx file change to be detected")
	}
}

func TestFileWatcher_ScanExcludesNodeModules(t *testing.T) {
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
	var goPending, fePending bool
	w.scan(true, &goPending, &fePending, nil, nil)

	// Modify after first scan.
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(path, []byte("package bar\n"), 0644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	goPending, fePending = false, false
	w.scan(false, &goPending, &fePending, nil, nil)
	if goPending || fePending {
		t.Fatal("node_modules should be excluded from watching")
	}
}

func TestFileWatcher_ScanRemovesDeletedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	w := newFileWatcher(dir)
	var goPending, fePending bool
	w.scan(true, &goPending, &fePending, nil, nil)
	if _, ok := w.mtimes[path]; !ok {
		t.Fatal("expected file to be tracked after first scan")
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("failed to remove file: %v", err)
	}

	goPending, fePending = false, false
	w.scan(false, &goPending, &fePending, nil, nil)
	if _, ok := w.mtimes[path]; ok {
		t.Fatal("expected deleted file to be removed from mtimes")
	}
}
