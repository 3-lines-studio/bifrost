package bifrost

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Compress wraps a handler with gzip for the responses that shrink and the
// clients that ask for it.
//
// The allowlist is by content type: a document, a stylesheet, a script, JSON,
// SVG, or Markdown. Fonts, images, and video are already compressed, and a
// second pass would cost CPU to make them bigger. A response that already
// carries a Content-Encoding, or a Cache-Control that says no-transform, is
// passed through untouched, and so is an event stream, whose frames have to
// reach the browser one at a time.
//
// The body is compressed as it goes, so the length is no longer known: the
// header is dropped and the response becomes chunked. A request that arrives
// through Cloudflare carries a CF-Ray header: Cloudflare compresses with Brotli
// and keeps whatever encoding the origin sends, so those requests are left
// alone. The browser gets the smaller Brotli, and the bytes between the edge and
// the origin are Cloudflare's to save.
func Compress(next http.Handler) http.Handler {
	if next == nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if throughCloudflare(r) || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		writer := &gzipResponseWriter{ResponseWriter: w}
		defer writer.close()
		next.ServeHTTP(writer, r)
	})
}

func throughCloudflare(r *http.Request) bool {
	return r.Header.Get("CF-Ray") != ""
}

const minimumCompressedBytes = 1024

var gzipWriters = sync.Pool{
	New: func() any {
		writer, err := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		if err != nil {
			return nil
		}
		return writer
	},
}

var compressibleTypes = map[string]bool{
	"text/html":                 true,
	"text/css":                  true,
	"text/plain":                true,
	"text/markdown":             true,
	"text/javascript":           true,
	"text/xml":                  true,
	"application/javascript":    true,
	"application/json":          true,
	"application/ld+json":       true,
	"application/manifest+json": true,
	"application/xml":           true,
	"application/rss+xml":       true,
	"application/atom+xml":      true,
	"image/svg+xml":             true,
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		coding, parameters, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(coding), "gzip") {
			continue
		}
		quality := 1.0
		for _, parameter := range strings.Split(parameters, ";") {
			value, found := strings.CutPrefix(strings.TrimSpace(parameter), "q=")
			if !found {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil {
				continue
			}
			quality = parsed
		}
		return quality > 0
	}
	return false
}

func compressibleContentType(header string) bool {
	contentType, _, _ := strings.Cut(header, ";")
	return compressibleTypes[strings.ToLower(strings.TrimSpace(contentType))]
}

// gzipResponseWriter decides once, when the status and the headers are known,
// whether the response is compressed, and then streams it through a pooled
// writer.
type gzipResponseWriter struct {
	http.ResponseWriter
	status   int
	decided  bool
	compress bool
	writer   *gzip.Writer
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.decided {
		return
	}
	w.decide(status)
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	if !w.decided {
		w.decide(http.StatusOK)
	}
	if !w.compress {
		return w.ResponseWriter.Write(data)
	}
	if w.writer == nil {
		w.start()
	}
	return w.writer.Write(data)
}

func (w *gzipResponseWriter) decide(status int) {
	w.decided = true
	header := w.ResponseWriter.Header()
	w.status = status

	switch {
	case status == http.StatusNoContent, status == http.StatusNotModified, status < 200:
	case header.Get("Content-Encoding") != "":
	case strings.Contains(header.Get("Cache-Control"), "no-transform"):
	case !compressibleContentType(header.Get("Content-Type")):
	default:
		if length := header.Get("Content-Length"); length != "" {
			size, err := strconv.Atoi(length)
			if err != nil || size < minimumCompressedBytes {
				return
			}
		}
		w.compress = true
	}

	if !w.compress {
		return
	}
	header.Set("Content-Encoding", "gzip")
	header.Del("Content-Length")
	header.Add("Vary", "Accept-Encoding")
}

func (w *gzipResponseWriter) start() {
	pooled, _ := gzipWriters.Get().(*gzip.Writer)
	if pooled == nil {
		pooled = gzip.NewWriter(io.Discard)
	}
	pooled.Reset(w.ResponseWriter)
	w.writer = pooled
}

// Flush hands what is buffered to the client. A compressed response flushes the
// compressor first, so a stream keeps its frames.
func (w *gzipResponseWriter) Flush() {
	if w.writer != nil {
		_ = w.writer.Flush()
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *gzipResponseWriter) close() {
	if w.writer == nil {
		return
	}
	_ = w.writer.Close()
	w.writer.Reset(io.Discard)
	gzipWriters.Put(w.writer)
	w.writer = nil
}
