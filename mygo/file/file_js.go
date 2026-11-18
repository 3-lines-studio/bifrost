//go:build js && wasm

package file

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"sync"
	"syscall/js"
	"time"

	"github.com/3-lines-studio/bifrost/mygo/client"
	"github.com/3-lines-studio/bifrost/mygo/internal/uithread"
)

// ErrUIThread is what a call that waits for the browser answers when it is made
// where the app draws: the person picks, and the browser answers, on that
// goroutine, so one that waits for it there waits forever.
var ErrUIThread = client.ErrUIThread

// File is a file of the person using the app: one the browser holds, as the
// picker hands over, or one the browser keeps at a path, as a file dropped on
// the window, which os.Open reads.
type File struct {
	picked js.Value
	handle *os.File
	name   string
	size   int64
	mod    time.Time
	mime   string
}

// Open returns the file at path, as one the window dropped on it: browsers give
// dropped files paths of their own, which os.Open reads.
func Open(name string) (*File, error) {
	handle, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	stat, err := handle.Stat()
	if err != nil {
		handle.Close()
		return nil, err
	}
	return &File{handle: handle, name: path.Base(name), size: stat.Size(), mod: stat.ModTime()}, nil
}

// Pick asks the person for files: the browser shows its picker.
func Pick(ctx context.Context, options Options) ([]*File, error) {
	if uithread.On() {
		return nil, ErrUIThread
	}
	input := newInput(options)
	defer input.Call("remove")

	answer := make(chan picked, 1)
	var onChange, onCancel js.Func
	var once sync.Once
	release := func() {
		once.Do(func() {
			onChange.Release()
			onCancel.Release()
		})
	}
	finish := func(value picked) {
		release()
		answer <- value
	}
	onChange = js.FuncOf(func(js.Value, []js.Value) any {
		files := input.Get("files")
		count := files.Length()
		if count == 0 {
			finish(picked{})
			return nil
		}
		chosen := make([]js.Value, count)
		for i := range chosen {
			chosen[i] = files.Index(i)
		}
		finish(picked{files: chosen})
		return nil
	})
	onCancel = js.FuncOf(func(js.Value, []js.Value) any {
		finish(picked{})
		return nil
	})
	input.Call("addEventListener", "change", onChange)
	input.Call("addEventListener", "cancel", onCancel)
	input.Call("click")

	select {
	case chosen := <-answer:
		files := make([]*File, 0, len(chosen.files))
		for _, value := range chosen.files {
			files = append(files, fromBrowser(value))
		}
		return files, nil
	case <-ctx.Done():
		input.Call("removeEventListener", "change", onChange)
		input.Call("removeEventListener", "cancel", onCancel)
		release()
		return nil, ctx.Err()
	}
}

// picked is what the picker answered: no files when the person canceled.
type picked struct {
	files []js.Value
}

// newInput returns the file input the browser shows its picker for, out of the
// page.
func newInput(options Options) js.Value {
	document := js.Global().Get("document")
	input := document.Call("createElement", "input")
	input.Set("type", "file")
	if options.Multiple {
		input.Set("multiple", true)
	}
	if accepted := accept(options.Filters); accepted != "" {
		input.Set("accept", accepted)
	}
	input.Set("style", "position:fixed;left:-10000px;top:0;width:1px;height:1px;opacity:0")
	document.Get("body").Call("appendChild", input)
	return input
}

// fromBrowser returns the File of a file the browser handed over.
func fromBrowser(value js.Value) *File {
	file := &File{
		picked: value,
		name:   value.Get("name").String(),
		size:   int64(value.Get("size").Int()),
		mime:   value.Get("type").String(),
	}
	if modified := value.Get("lastModified"); modified.Type() == js.TypeNumber {
		file.mod = time.UnixMilli(int64(modified.Int()))
	}
	return file
}

// Name is the name of the file, not its path.
func (f *File) Name() string { return f.name }

// Size is how many bytes the file holds.
func (f *File) Size() int64 { return f.size }

// ModTime is when the file last changed.
func (f *File) ModTime() time.Time { return f.mod }

// ContentType is the media type of the file: the browser's when it has one, the
// one of its extension otherwise.
func (f *File) ContentType() string { return contentType(f.name, f.mime) }

// Head reads the first n bytes of the file, or all of them when it is shorter.
func (f *File) Head(ctx context.Context, n int64) ([]byte, error) {
	if n > f.size {
		n = f.size
	}
	if f.handle != nil {
		return head(ctx, f.handle, f.size, n)
	}
	if f.picked.Type() != js.TypeObject {
		return nil, errors.New("file: the file is gone")
	}
	if uithread.On() {
		return nil, ErrUIThread
	}
	buffer, err := await(f.picked.Call("slice", 0, n).Call("arrayBuffer"))
	if err != nil {
		return nil, err
	}
	array := js.Global().Get("Uint8Array").New(buffer)
	data := make([]byte, array.Length())
	js.CopyBytesToGo(data, array)
	return data, nil
}

// Blob returns the bytes from start to end of the file, as a request sends
// them: the browser reads a file it holds as the request goes.
func (f *File) Blob(start, end int64) client.Blob {
	if !validRange(start, end, f.size) {
		panic("file: range outside the file")
	}
	if f.handle != nil {
		return client.ReaderBlob(f.handle, f.size).Slice(start, end)
	}
	return client.FileBlob(f.picked, f.size).Slice(start, end)
}

// Close closes the file. A file the browser holds goes when the person closes
// the page, or when nothing keeps it: Close forgets it here.
func (f *File) Close() error {
	f.picked = js.Undefined()
	if f.handle != nil {
		return f.handle.Close()
	}
	return nil
}

// Save has the browser download the file, as it downloads any link. The URL
// authorizes the download itself, as a signed URL does, or the cookies of the
// page do; headers the app would send do not go with it.
func Save(ctx context.Context, download Download) error {
	document := js.Global().Get("document")
	link := document.Call("createElement", "a")
	link.Set("href", download.URL)
	link.Set("download", download.Name)
	link.Set("rel", "noopener")
	document.Get("body").Call("appendChild", link)
	link.Call("click")
	link.Call("remove")
	return nil
}

// head reads the first n bytes of a file of size bytes, from the ones the
// browser keeps at a path.
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

// await waits for the promise value to settle, and returns what it answered.
// It is called from a goroutine: the promise answers on the one that draws.
func await(value js.Value) (js.Value, error) {
	settled := make(chan js.Value, 1)
	failed := make(chan error, 1)
	var onValue, onError js.Func
	release := func() {
		onValue.Release()
		onError.Release()
	}
	onValue = js.FuncOf(func(_ js.Value, args []js.Value) any {
		release()
		settled <- args[0]
		return nil
	})
	onError = js.FuncOf(func(_ js.Value, args []js.Value) any {
		release()
		failed <- errors.New("file: the browser could not read the file")
		return nil
	})
	value.Call("then", onValue, onError)
	select {
	case settledValue := <-settled:
		return settledValue, nil
	case err := <-failed:
		return js.Undefined(), err
	}
}
