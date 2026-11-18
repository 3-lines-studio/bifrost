//go:build !windows

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestPruneStaleDevelopmentFilesRemovesUnlockedRuns(t *testing.T) {
	base := t.TempDir()
	stale := "bifrost-dev-1111111111111111"
	live := "bifrost-dev-2222222222222222"
	current := "bifrost-dev-3333333333333333"
	orphan := "bifrost-dev-4444444444444444"
	for _, root := range []string{filepath.Join(base, stale+"-1234"), filepath.Join(base, orphan)} {
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(base, stale+".lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	liveLock, err := os.OpenFile(filepath.Join(base, live+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = liveLock.Close() }()
	if err := syscall.Flock(int(liveLock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	pruneStaleDevelopmentFiles(base, current+".lock")

	for _, removed := range []string{filepath.Join(base, stale+".lock"), filepath.Join(base, stale+"-1234")} {
		if _, err := os.Stat(removed); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%s survived: %v", removed, err)
		}
	}
	for _, kept := range []string{filepath.Join(base, live+".lock"), filepath.Join(base, orphan)} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("%s was removed: %v", kept, err)
		}
	}
}

func TestDevWatchesTheWholeTree(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "store")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	notes := filepath.Join(store, "store.go")
	if err := os.WriteFile(notes, []byte("package store\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := goTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	same, err := goTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if first != same {
		t.Fatal("stamp changed without a change")
	}
	later := time.Now().Add(time.Second)
	if err := os.Chtimes(notes, later, later); err != nil {
		t.Fatal(err)
	}
	changed, err := goTreeStamp(root)
	if err != nil {
		t.Fatal(err)
	}
	if changed == first {
		t.Fatal("a package beside app/ does not restart the server")
	}
}
