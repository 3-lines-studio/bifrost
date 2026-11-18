//go:build !js

package client

import (
	"bytes"
	"io"
)

// Blob is a range of a file or of memory that a request sends as its body.
// A request with a Blob sends it where it is: a file is read as the request
// goes, not copied into another buffer first.
//
// The zero Blob is no body. BytesBlob and ReaderBlob make one, and Slice takes
// a part of it, as one part of a multipart upload.
type Blob struct {
	r     io.ReaderAt
	size  int64
	start int64
	end   int64
}

// BytesBlob returns a Blob of b. b must not change until the request that
// sends it returns.
func BytesBlob(b []byte) Blob {
	return Blob{r: bytes.NewReader(b), size: int64(len(b)), end: int64(len(b))}
}

// ReaderBlob returns a Blob of the size bytes r holds.
func ReaderBlob(r io.ReaderAt, size int64) Blob {
	if size < 0 {
		panic("client: ReaderBlob of a negative size")
	}
	return Blob{r: r, size: size, end: size}
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
	return Blob{r: b.r, size: b.size, start: b.start + start, end: b.start + end}
}

func (b Blob) present() bool { return b.r != nil }

func (b Blob) open() (io.ReadCloser, int64) {
	length := b.end - b.start
	return io.NopCloser(io.NewSectionReader(b.r, b.start, length)), length
}
