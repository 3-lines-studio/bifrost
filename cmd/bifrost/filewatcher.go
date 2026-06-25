package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounceDelay = 200 * time.Millisecond

type fileWatcher struct {
	fsw         *fsnotify.Watcher
	root        string
	excludeDirs map[string]bool
	goExts      map[string]bool
	feExts      map[string]bool
}

func newFileWatcher(root string) *fileWatcher {
	return &fileWatcher{
		root:        root,
		excludeDirs: map[string]bool{"node_modules": true, ".bifrost": true, "tmp": true, ".git": true},
		goExts:      map[string]bool{".go": true},
		feExts:      map[string]bool{".tsx": true, ".ts": true, ".css": true},
	}
}

func (w *fileWatcher) addDirs(root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if root != path && w.excludeDirs[name] {
				return filepath.SkipDir
			}
			if err := w.fsw.Add(path); err != nil {
				fmt.Fprintf(os.Stderr, "watch: %s: %v\n", path, err)
			}
		}
		return nil
	})
}

func (w *fileWatcher) classify(name string) (goChange, feChange bool) {
	if strings.HasSuffix(name, "_test.go") {
		return false, false
	}
	ext := filepath.Ext(name)
	return w.goExts[ext], w.feExts[ext]
}

func (w *fileWatcher) Start(ctx context.Context) (goChanges, feChanges <-chan struct{}) {
	goCh := make(chan struct{}, 1)
	feCh := make(chan struct{}, 1)
	goChanges = goCh
	feChanges = feCh

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		close(goCh)
		close(feCh)
		return goChanges, feChanges
	}
	w.fsw = fsw

	go func() {
		defer close(goCh)
		defer close(feCh)
		defer func() { _ = w.fsw.Close() }()

		w.addDirs(w.root)

		goTimer := time.NewTimer(debounceDelay)
		goTimer.Stop()
		feTimer := time.NewTimer(debounceDelay)
		feTimer.Stop()

		for {
			select {
			case <-ctx.Done():
				goTimer.Stop()
				feTimer.Stop()
				return

			case ev, ok := <-w.fsw.Events:
				if !ok {
					return
				}
				if ev.Has(fsnotify.Create) {
					if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
						w.addDirs(ev.Name)
						continue
					}
				}
				if !ev.Has(fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename) {
					continue
				}
				isGo, isFe := w.classify(filepath.Base(ev.Name))
				if isGo {
					if !goTimer.Stop() {
						select {
						case <-goTimer.C:
						default:
						}
					}
					goTimer.Reset(debounceDelay)
				} else if isFe {
					if !feTimer.Stop() {
						select {
						case <-feTimer.C:
						default:
						}
					}
					feTimer.Reset(debounceDelay)
				}

			case <-goTimer.C:
				select {
				case goCh <- struct{}{}:
				default:
				}

			case <-feTimer.C:
				select {
				case feCh <- struct{}{}:
				default:
				}

			case err, ok := <-w.fsw.Errors:
				if !ok {
					return
				}
				if err != nil {
					fmt.Fprintf(os.Stderr, "watch error: %v\n", err)
				}
			}
		}
	}()
	return goChanges, feChanges
}
