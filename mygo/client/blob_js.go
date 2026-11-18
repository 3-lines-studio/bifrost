//go:build js && wasm

package client

import (
	"errors"
	"io"
	"syscall/js"
)

// Blob is a range of a file or of memory that a request sends as its body.
// A request with a Blob sends it where it is: the browser reads a File the
// person picked as the request goes, not copied into Go first.
//
// The zero Blob is no body. BytesBlob and ReaderBlob make one, and Slice takes
// a part of it, as one part of a multipart upload.
type Blob struct {
	file  js.Value
	r     io.ReaderAt
	size  int64
	start int64
	end   int64
}

// BytesBlob returns a Blob of b.
func BytesBlob(b []byte) Blob {
	return Blob{r: readerAt(b), size: int64(len(b)), end: int64(len(b))}
}

// ReaderBlob returns a Blob of the size bytes r holds.
func ReaderBlob(r io.ReaderAt, size int64) Blob {
	if size < 0 {
		panic("client: ReaderBlob of a negative size")
	}
	return Blob{r: r, size: size, end: size}
}

// FileBlob returns a Blob of the file the browser handed over, as one the
// person picked or dropped: it is sent by the browser, which reads only the
// part a request asks for.
func FileBlob(file js.Value, size int64) Blob {
	return Blob{file: file, size: size, end: size}
}

// Size is how many bytes the Blob sends.
func (b Blob) Size() int64 { return b.end - b.start }

// Slice returns the bytes from start to end of the Blob, as a Blob of its own.
// It panics when the range is not inside the Blob: a range past its end is a
// mistake in the code that asks for it, not a condition to handle.
func (b Blob) Slice(start, end int64) Blob {
	if start < 0 || end < start || end > b.size {
		panic("client: blob slice outside the blob")
	}
	return Blob{file: b.file, r: b.r, size: b.size, start: b.start + start, end: b.start + end}
}

func (b Blob) present() bool { return b.file.Type() == js.TypeObject || b.r != nil }

// body returns what the browser sends: the sliced File when the Blob holds one,
// and the bytes themselves when it holds memory.
func (b Blob) body() (js.Value, error) {
	if b.file.Type() == js.TypeObject {
		return b.file.Call("slice", b.start, b.end), nil
	}
	if b.r == nil {
		return js.Undefined(), errNoBlob
	}
	data := make([]byte, b.end-b.start)
	if _, err := b.r.ReadAt(data, b.start); err != nil && err != io.EOF {
		return js.Undefined(), err
	}
	array := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(array, data)
	return array, nil
}

var errNoBlob = errors.New("client: blob has no bytes")

// readerAt reads a byte slice at an offset, as io.ReaderAt.
type readerAt []byte

func (r readerAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r)) {
		return 0, io.EOF
	}
	return copy(p, r[off:]), nil
}
