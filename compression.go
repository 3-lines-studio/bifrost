package bifrost

import (
	"compress/gzip"
	"net/http"
	"sync"
)

var gzipWriters = sync.Pool{New: func() any {
	writer, err := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
	if err != nil {
		return nil
	}
	return writer
}}

// documentWriter compresses a rendered document or navigation payload with
// gzip when the client accepts it. It stays a no-op for every other request,
// so handlers keep writing into the same interface.
type documentWriter struct {
	writer     http.ResponseWriter
	gzip       *gzip.Writer
	compressed bool
	started    bool
	finished   bool
}

func newDocumentWriter(writer http.ResponseWriter, request *http.Request) *documentWriter {
	compressed := request.Method != http.MethodHead && acceptedEncodings(request.Header.Get("Accept-Encoding"))["gzip"] > 0
	return &documentWriter{writer: writer, compressed: compressed}
}

func (d *documentWriter) Header() http.Header { return d.writer.Header() }

func (d *documentWriter) WriteHeader(status int) {
	if d.started {
		return
	}
	d.started = true
	if !d.compressed {
		d.writer.WriteHeader(status)
		return
	}
	d.writer.Header().Add("Vary", "Accept-Encoding")
	d.writer.Header().Set("Content-Encoding", "gzip")
	d.writer.Header().Del("Content-Length")
	d.writer.WriteHeader(status)
	writer := gzipWriters.Get()
	if writer == nil {
		return
	}
	stream := writer.(*gzip.Writer)
	stream.Reset(d.writer)
	d.gzip = stream
}

func (d *documentWriter) Write(body []byte) (int, error) {
	if !d.started {
		d.WriteHeader(http.StatusOK)
	}
	if d.gzip == nil {
		return d.writer.Write(body)
	}
	return d.gzip.Write(body)
}

func (d *documentWriter) Flush() {
	if d.gzip != nil {
		_ = d.gzip.Flush()
	}
	if flusher, ok := d.writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (d *documentWriter) Unwrap() http.ResponseWriter { return d.writer }

// Close terminates the gzip stream and returns the writer to the pool. It is
// safe to call more than once.
func (d *documentWriter) Close() {
	if d.finished {
		return
	}
	d.finished = true
	if d.gzip == nil {
		return
	}
	if err := d.gzip.Close(); err == nil {
		gzipWriters.Put(d.gzip)
	}
	d.gzip = nil
}
