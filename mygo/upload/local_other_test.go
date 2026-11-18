//go:build !js

package upload

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDiskStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uploads")
	store := Disk(dir)
	if _, ok := store.Load("photo.jpg:10"); ok {
		t.Fatal("an empty store had a state")
	}
	state := State{UploadID: "upload-1", PartSize: 4, Size: 10, Parts: []Part{{Number: 1, ETag: "etag-1", Size: 4}}}
	if err := store.Save("photo.jpg:10", state); err != nil {
		t.Fatal(err)
	}
	loaded, ok := store.Load("photo.jpg:10")
	if !ok {
		t.Fatal("the state did not come back")
	}
	if loaded.UploadID != "upload-1" || len(loaded.Parts) != 1 || loaded.Parts[0].ETag != "etag-1" {
		t.Fatalf("state %+v", loaded)
	}
	if err := store.Save("photo.jpg:10", State{UploadID: "upload-2", Size: 10}); err != nil {
		t.Fatal(err)
	}
	if loaded, _ := store.Load("photo.jpg:10"); loaded.UploadID != "upload-2" {
		t.Fatalf("state %+v", loaded)
	}
	if err := store.Delete("photo.jpg:10"); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Load("photo.jpg:10"); ok {
		t.Fatal("a deleted state came back")
	}
	if err := store.Delete("photo.jpg:10"); err != nil {
		t.Fatalf("deleting a state twice: %v", err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("the directory holds %d files, error %v", len(entries), err)
	}
}

func TestDiskStoreConcurrent(t *testing.T) {
	store := Disk(t.TempDir())
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for n := 0; n < 20; n++ {
				if err := store.Save("key", State{UploadID: "upload", Size: int64(n)}); err != nil {
					t.Errorf("save: %v", err)
					return
				}
				store.Load("key")
			}
		}()
	}
	group.Wait()
	if _, ok := store.Load("key"); !ok {
		t.Fatal("no state after the writes")
	}
	if entries, err := os.ReadDir(store.(*disk).dir); err != nil || len(entries) != 1 {
		t.Fatalf("the directory holds %d files, error %v", len(entries), err)
	}
}

func TestRunResumesFromDisk(t *testing.T) {
	server := newStore()
	server.fail[2] = 100
	fake := &signer{store: server, url: server.server(t).URL, part: 4}
	file := memoryFile{name: "photo.jpg", data: []byte("0123456789")}
	store := Disk(t.TempDir())
	if _, err := Run(t.Context(), file, fake, Options{Store: store, Retry: Retry{Attempts: 1}, Concurrency: 1}); err == nil {
		t.Fatal("a part that always fails did not fail")
	}
	if _, ok := store.Load("photo.jpg:10"); !ok {
		t.Fatal("the upload was not kept on disk")
	}
	server.mu.Lock()
	server.fail = map[int]int{}
	server.mu.Unlock()
	result, err := Run(t.Context(), file, fake, Options{Store: store})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Parts) != 3 {
		t.Fatalf("parts %+v", result.Parts)
	}
}
