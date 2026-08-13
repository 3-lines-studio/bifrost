package bifrost

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/3-lines-studio/bifrost/internal/dochtml"
	"github.com/3-lines-studio/bifrost/internal/protocol"
)

type benchmarkWriter struct {
	header http.Header
	status int
	bytes  int
}

func newBenchmarkWriter() *benchmarkWriter {
	return &benchmarkWriter{header: make(http.Header, 4)}
}
func (w *benchmarkWriter) Header() http.Header    { return w.header }
func (w *benchmarkWriter) WriteHeader(status int) { w.status = status }
func (w *benchmarkWriter) Write(data []byte) (int, error) {
	w.bytes += len(data)
	return len(data), nil
}
func (w *benchmarkWriter) reset() { w.status, w.bytes = 0, 0 }

func BenchmarkClientHandler(b *testing.B) {
	handler := &clientPageHandler{document: []byte("<!doctype html><html><body>client</body></html>"), contentLength: "47"}
	request, _ := http.NewRequest(http.MethodGet, "/app", nil)
	writer := newBenchmarkWriter()
	b.ReportAllocs()
	for b.Loop() {
		writer.reset()
		handler.ServeHTTP(writer, request)
	}
}

func BenchmarkClientHandlerWithHook(b *testing.B) {
	client := &clientPageHandler{document: []byte("<!doctype html><html><body>client</body></html>"), contentLength: "47"}
	handler := &responseObserver{pattern: "/app", next: client, hooks: []ResponseHook{func(context.Context, ResponseEvent) {}}}
	request, _ := http.NewRequest(http.MethodGet, "/app", nil)
	writer := newBenchmarkWriter()
	b.ReportAllocs()
	for b.Loop() {
		writer.reset()
		handler.ServeHTTP(writer, request)
	}
}

func BenchmarkPlainHTTPHandler(b *testing.B) {
	document := []byte("<!doctype html><html><body>client</body></html>")
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Length", "47")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(document)
	})
	request, _ := http.NewRequest(http.MethodGet, "/app", nil)
	writer := newBenchmarkWriter()
	b.ReportAllocs()
	for b.Loop() {
		writer.reset()
		handler.ServeHTTP(writer, request)
	}
}

func BenchmarkDynamicStaticLookup(b *testing.B) {
	for _, size := range []int{1, 100, 10_000} {
		b.Run(fmt.Sprintf("paths_%d", size), func(b *testing.B) {
			files := make(map[string]protocol.FileRef, size)
			assets := fstest.MapFS{}
			for i := range size {
				requestPath := fmt.Sprintf("/page/%d", i)
				filePath := fmt.Sprintf("pages/%d.html", i)
				files[requestPath] = protocol.FileRef{Path: filePath, Size: 1}
				assets[filePath] = &fstest.MapFile{Data: []byte("x")}
			}
			handler := &staticPageHandler{assets: assets, files: files}
			request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/page/%d", size-1), nil)
			writer := newBenchmarkWriter()
			b.ReportAllocs()
			for b.Loop() {
				writer.reset()
				handler.ServeHTTP(writer, request)
			}
		})
	}
}

type benchmarkRenderer struct{}

func (benchmarkRenderer) Render(_ context.Context, _ renderRequest, sink renderSink) error {
	if err := sink.Head(nil); err != nil {
		return err
	}
	return sink.Body([]byte("<main>hello</main>"))
}
func (benchmarkRenderer) Close(context.Context) error { return nil }

func BenchmarkServerHandlerGoOverhead(b *testing.B) {
	shell, _ := dochtml.NewShell(protocol.AssetSet{Entry: protocol.FileRef{Path: "dist/app.js"}})
	handler := &serverPageHandler{pattern: "/", shell: shell, entry: "ssr/app.js", render: benchmarkRenderer{}, limits: Limits{MaxPropsBytes: defaultMaxPropsBytes, MaxHeadBytes: defaultMaxHeadBytes, MaxFrameBytes: defaultMaxFrameBytes}}
	request, _ := http.NewRequest(http.MethodGet, "/", nil)
	writer := newBenchmarkWriter()
	b.ReportAllocs()
	for b.Loop() {
		writer.reset()
		handler.ServeHTTP(writer, request)
	}
}

func BenchmarkDocumentStreaming(b *testing.B) {
	shell, _ := dochtml.NewShell(protocol.AssetSet{Entry: protocol.FileRef{Path: "dist/app.js"}})
	props, _ := marshalProps(map[string]any{"name": "Don", "count": 42})
	b.ReportAllocs()
	b.SetBytes(int64(len(props)))
	for b.Loop() {
		if err := shell.WritePreamble(io.Discard, []byte("<title>Home</title>"), protocol.DocumentAttributes{Lang: "en"}); err != nil {
			b.Fatal(err)
		}
		if err := shell.WriteSuffix(io.Discard, props); err != nil {
			b.Fatal(err)
		}
	}
}
