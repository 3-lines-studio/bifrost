package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
)

const (
	gzipEncoding   = "gzip"
	brotliEncoding = "br"
)

// FileServer serves the browser build beside the pages: /app.wasm and
// /wasm_exec.js read from dir. It is FileServerFS over the directory.
func FileServer(dir string) http.Handler {
	return FileServerFS(os.DirFS(dir))
}

// FileServerFS serves the browser build from files, a disk directory as
// os.DirFS gives it or a build embedded in the binary. It answers each file
// with the brotli or gzip build beside it, app.wasm.br or app.wasm.gz, which a
// build writes once; without one it compresses the file itself, once per
// encoding. It prefers brotli over gzip, and answers 304 to a matching ETag, so
// a reload costs a request. A file that changes, as during development, is
// read again.
func FileServerFS(files fs.FS) http.Handler {
	server := &fileServer{
		files: files,
		assets: map[string]*asset{
			execPath: {name: "wasm_exec.js", contentType: "text/javascript; charset=utf-8"},
			wasmPath: {name: "app.wasm", contentType: "application/wasm", compress: true},
		},
	}
	go server.warm()
	return server
}

// warm compresses the build in the background, so the first request that asks
// for an encoding the build did not write finds it ready.
func (s *fileServer) warm() {
	for _, file := range s.assets {
		if !file.compress {
			continue
		}
		if _, _, ok := file.load(s.files); !ok {
			continue
		}
		_, _ = file.encode(brotliEncoding)
		_, _ = file.encode(gzipEncoding)
	}
}

type fileServer struct {
	files  fs.FS
	assets map[string]*asset
}

type asset struct {
	name        string
	contentType string
	compress    bool

	mu      sync.Mutex
	modTime time.Time
	size    int64
	data    []byte
	digest  string
	missing bool
	encoded map[string][]byte
}

func (s *fileServer) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	file := s.assets[request.URL.Path]
	if file == nil {
		http.NotFound(w, request)
		return
	}
	s.serve(w, request, file)
}

func (s *fileServer) serve(w http.ResponseWriter, request *http.Request, file *asset) {
	data, digest, ok := file.load(s.files)
	if !ok {
		http.NotFound(w, request)
		return
	}
	w.Header().Set("ETag", digest)
	w.Header().Set("Cache-Control", "no-cache")
	if request.Header.Get("If-None-Match") == digest {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", file.contentType)
	body := data
	if file.compress {
		w.Header().Set("Vary", "Accept-Encoding")
		if encoding := acceptedEncoding(request); encoding != "" {
			if compressed, err := file.encode(encoding); err == nil {
				w.Header().Set("Content-Encoding", encoding)
				body = compressed
			}
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if request.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}

// sidecars are the encodings a build leaves beside the file, so serving one
// costs a read and not a compression.
var sidecars = map[string]string{
	brotliEncoding: ".br",
	gzipEncoding:   ".gz",
}

// load returns the file and its ETag, reading it again when it changed. It
// reports false when the file is missing.
func (a *asset) load(files fs.FS) ([]byte, string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	info, err := fs.Stat(files, a.name)
	if err != nil {
		a.missing, a.data, a.digest, a.encoded = true, nil, "", nil
		return nil, "", false
	}
	if a.missing || a.data == nil || !info.ModTime().Equal(a.modTime) || info.Size() != a.size {
		data, err := fs.ReadFile(files, a.name)
		if err != nil {
			a.missing = true
			return nil, "", false
		}
		sum := sha256.Sum256(data)
		a.missing, a.data, a.digest = false, data, `"`+hex.EncodeToString(sum[:])+`"`
		a.modTime, a.size, a.encoded = info.ModTime(), info.Size(), nil
		for encoding, suffix := range sidecars {
			sidecar, err := fs.ReadFile(files, a.name+suffix)
			if err != nil {
				continue
			}
			if a.encoded == nil {
				a.encoded = make(map[string][]byte, len(sidecars))
			}
			a.encoded[encoding] = sidecar
		}
	}
	return a.data, a.digest, true
}

func (a *asset) encode(encoding string) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if compressed, ok := a.encoded[encoding]; ok {
		return compressed, nil
	}
	if a.data == nil {
		return nil, os.ErrNotExist
	}
	compressed, err := compress(encoding, a.data)
	if err != nil {
		return nil, err
	}
	if a.encoded == nil {
		a.encoded = make(map[string][]byte, 2)
	}
	a.encoded[encoding] = compressed
	return compressed, nil
}

func compress(encoding string, data []byte) ([]byte, error) {
	var buffer bytes.Buffer
	if encoding == brotliEncoding {
		writer := brotli.NewWriterLevel(&buffer, brotli.DefaultCompression)
		if _, err := writer.Write(data); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		return buffer.Bytes(), nil
	}
	writer, err := gzip.NewWriterLevel(&buffer, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// acceptedEncoding returns the best encoding the request accepts, "" for none.
func acceptedEncoding(request *http.Request) string {
	qualities := map[string]float64{}
	any := 0.0
	for _, value := range strings.Split(request.Header.Get("Accept-Encoding"), ",") {
		coding, parameters, _ := strings.Cut(strings.TrimSpace(value), ";")
		coding = strings.ToLower(strings.TrimSpace(coding))
		if coding == "" {
			continue
		}
		quality := 1.0
		if index := strings.Index(parameters, "q="); index >= 0 {
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(parameters[index+2:]), 64); err == nil {
				quality = parsed
			}
		}
		if coding == "*" {
			any = quality
			continue
		}
		qualities[coding] = quality
	}
	quality := func(coding string) float64 {
		if value, ok := qualities[coding]; ok {
			return value
		}
		return any
	}
	brotli, gzipped := quality(brotliEncoding), quality(gzipEncoding)
	switch {
	case brotli > 0 && brotli >= gzipped:
		return brotliEncoding
	case gzipped > 0:
		return gzipEncoding
	}
	return ""
}
