//go:build !js

package file

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/3-lines-studio/bifrost/mygo/client"
	"github.com/egoist/mygo"
)

// ErrUIThread is what a call that waits for the browser answers when it is made
// where the app draws. A window has no such goroutine, so it never answers it.
var ErrUIThread = errors.New("file: nothing waits for the browser outside it")

// File is a file of the disk.
type File struct {
	handle *os.File
	name   string
	size   int64
	mod    time.Time
}

// Open returns the file at path.
func Open(path string) (*File, error) {
	handle, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	stat, err := handle.Stat()
	if err != nil {
		handle.Close()
		return nil, err
	}
	return &File{handle: handle, name: stat.Name(), size: stat.Size(), mod: stat.ModTime()}, nil
}

// Pick asks the person for files in a dialog of the window.
func Pick(ctx context.Context, options Options) ([]*File, error) {
	open := mygo.OpenDialogOptions{
		Title:    options.Title,
		Filters:  options.Filters,
		Multiple: options.Multiple,
	}
	paths, err := mygo.Dialog.Open(open)
	if err != nil {
		return nil, err
	}
	files := make([]*File, 0, len(paths))
	for _, path := range paths {
		file, err := Open(path)
		if err != nil {
			closeAll(files)
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

// Name is the name of the file, not its path.
func (f *File) Name() string { return f.name }

// Size is how many bytes the file holds.
func (f *File) Size() int64 { return f.size }

// ModTime is when the file last changed.
func (f *File) ModTime() time.Time { return f.mod }

// ContentType is the media type of the file, from its extension.
func (f *File) ContentType() string { return contentType(f.name, "") }

// Head reads the first n bytes of the file, or all of them when it is shorter.
func (f *File) Head(ctx context.Context, n int64) ([]byte, error) {
	return head(ctx, f.handle, f.size, n)
}

// Blob returns the bytes from start to end of the file, as a request sends
// them: the disk is read as the request goes.
func (f *File) Blob(start, end int64) client.Blob {
	if !validRange(start, end, f.size) {
		panic("file: range outside the file")
	}
	return client.ReaderBlob(f.handle, f.size).Slice(start, end)
}

// Close closes the file.
func (f *File) Close() error { return f.handle.Close() }

// Save asks where to save the file and writes it there as it comes.
func Save(ctx context.Context, download Download) error {
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{DefaultPath: download.Name})
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	saved, err := os.Create(path)
	if err != nil {
		return err
	}
	defer saved.Close()
	request := client.Request{Method: "GET", URL: download.URL, Header: download.Header}
	if _, err := client.Copy(ctx, request, saved, download.OnProgress); err != nil {
		saved.Close()
		os.Remove(path)
		return err
	}
	return saved.Sync()
}

// head reads the first n bytes of a file of size bytes.
func head(ctx context.Context, reader io.ReaderAt, size, n int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if n > size {
		n = size
	}
	buffer := make([]byte, n)
	read, err := reader.ReadAt(buffer, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buffer[:read], nil
}

func closeAll(files []*File) {
	for _, file := range files {
		file.Close()
	}
}
