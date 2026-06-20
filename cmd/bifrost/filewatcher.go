package main

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

type fileWatcher struct {
	root        string
	excludeDirs map[string]bool
	goExts      map[string]bool
	feExts      map[string]bool
	interval    time.Duration
	mtimes      map[string]int64
}

func newFileWatcher(root string) *fileWatcher {
	return &fileWatcher{
		root:        root,
		excludeDirs: map[string]bool{"node_modules": true, ".bifrost": true, "tmp": true, ".git": true},
		goExts:      map[string]bool{".go": true},
		feExts:      map[string]bool{".tsx": true, ".ts": true, ".svelte": true, ".css": true},
		interval:    500 * time.Millisecond,
		mtimes:      make(map[string]int64),
	}
}

func (w *fileWatcher) Start(ctx context.Context) (goChanges, feChanges <-chan struct{}) {
	goCh := make(chan struct{}, 1)
	feCh := make(chan struct{}, 1)
	goChanges = goCh
	feChanges = feCh

	first := true
	go func() {
		defer close(goCh)
		defer close(feCh)
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()

		var goPending, fePending bool

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.scan(first, &goPending, &fePending, goCh, feCh)
				first = false
				if goPending {
					select {
					case goCh <- struct{}{}:
						goPending = false
					default:
					}
				}
				if fePending {
					select {
					case feCh <- struct{}{}:
						fePending = false
					default:
					}
				}
			}
		}
	}()
	return goChanges, feChanges
}

func (w *fileWatcher) scan(first bool, goPending, fePending *bool, goCh, feCh chan<- struct{}) {
	currentFiles := make(map[string]bool)

	_ = filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if w.excludeDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mtime := info.ModTime().UnixNano()
		currentFiles[path] = true

		old, exists := w.mtimes[path]
		if !first && exists && mtime != old {
			ext := filepath.Ext(name)
			if w.goExts[ext] {
				*goPending = true
			} else if w.feExts[ext] {
				*fePending = true
			}
		}
		w.mtimes[path] = mtime
		return nil
	})

	for p := range w.mtimes {
		if !currentFiles[p] {
			delete(w.mtimes, p)
		}
	}
}
