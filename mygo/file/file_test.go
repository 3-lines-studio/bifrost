//go:build !js

package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/3-lines-studio/bifrost/mygo/client"
	"github.com/egoist/mygo"
)

func written(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpen(t *testing.T) {
	path := written(t, "photo.jpg", []byte("0123456789"))
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if file.Name() != "photo.jpg" {
		t.Fatalf("name %q", file.Name())
	}
	if file.Size() != 10 {
		t.Fatalf("size %d", file.Size())
	}
	if file.ContentType() != "image/jpeg" {
		t.Fatalf("type %q", file.ContentType())
	}
	if file.ModTime().IsZero() {
		t.Fatal("no modification time")
	}
}

func TestHead(t *testing.T) {
	path := written(t, "notes.txt", []byte("0123456789"))
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	head, err := file.Head(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(head) != "0123" {
		t.Fatalf("head %q", head)
	}
	all, err := file.Head(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if string(all) != "0123456789" {
		t.Fatalf("all %q", all)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := file.Head(ctx, 1); err != context.Canceled {
		t.Fatalf("canceled head answered %v", err)
	}
}

func TestBlob(t *testing.T) {
	path := written(t, "photo.jpg", []byte("0123456789"))
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	blob := file.Blob(2, 6)
	if blob.Size() != 4 {
		t.Fatalf("size %d", blob.Size())
	}
	response, err := client.Do(context.Background(), client.Request{Method: "POST", URL: "http://example.invalid", Blob: blob})
	if err == nil {
		t.Fatalf("answered %v", response)
	}
}

func TestBlobOutside(t *testing.T) {
	path := written(t, "photo.jpg", []byte("0123"))
	file, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	defer func() {
		if recover() == nil {
			t.Fatal("a range past the end did not panic")
		}
	}()
	file.Blob(0, 5)
}

func TestContentTypeFallsBack(t *testing.T) {
	if got := contentType("file.unknownextension", ""); got != "application/octet-stream" {
		t.Fatalf("type %q", got)
	}
	if got := contentType("photo.jpg", "image/heic"); got != "image/heic" {
		t.Fatalf("type %q", got)
	}
}

func TestAccept(t *testing.T) {
	if got := accept(nil); got != "" {
		t.Fatalf("accept %q", got)
	}
	filters := []mygo.FileFilter{{MIMETypes: []string{"image/*"}, Extensions: []string{".jpg", "*.png"}}}
	if got := accept(filters); got != "image/*,.jpg,.png" {
		t.Fatalf("accept %q", got)
	}
}
