package upload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/3-lines-studio/bifrost/mygo/client"
)

// memoryFile is a file in memory, as one the app opened.
type memoryFile struct {
	name string
	data []byte
}

func (f memoryFile) Name() string        { return f.name }
func (f memoryFile) Size() int64         { return int64(len(f.data)) }
func (f memoryFile) ContentType() string { return "image/jpeg" }
func (f memoryFile) Blob(start, end int64) client.Blob {
	return client.BytesBlob(f.data).Slice(start, end)
}

// store is what the server of a test keeps of one upload.
type store struct {
	mu       sync.Mutex
	parts    map[int][]byte
	uploadID string
	complete bool
	aborted  bool
	fail     map[int]int
	active   int
	most     int
	requests atomic.Int32
}

func newStore() *store {
	return &store{parts: map[int][]byte{}, fail: map[int]int{}}
}

func (s *store) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /part", func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		s.mu.Lock()
		s.active++
		if s.active > s.most {
			s.most = s.active
		}
		fail := s.fail[atoi(r.URL.Query().Get("number"))]
		if fail > 0 {
			s.fail[atoi(r.URL.Query().Get("number"))] = fail - 1
			s.active--
			s.mu.Unlock()
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.active--
			s.mu.Unlock()
		}()
		number := atoi(r.URL.Query().Get("number"))
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read part: %v", err)
		}
		s.mu.Lock()
		s.parts[number] = data
		s.mu.Unlock()
		w.Header().Set("ETag", `"etag-`+strconv.Itoa(number)+`"`)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("PUT /whole", func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read file: %v", err)
		}
		s.mu.Lock()
		s.parts[0] = data
		s.mu.Unlock()
		w.Header().Set("ETag", `"whole-etag"`)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func atoi(value string) int {
	n, _ := strconv.Atoi(value)
	return n
}

// signer is the API of the server of a test.
type signer struct {
	store  *store
	url    string
	part   int64
	single bool
	// failSign makes Sign fail for the first calls.
	failSign int
	mu       sync.Mutex
	signed   [][]int
	complete []Part
	started  int
}

func (s *signer) Start(ctx context.Context, file Info) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started++
	if s.single {
		return Plan{URL: s.url + "/whole"}, nil
	}
	s.store.mu.Lock()
	s.store.uploadID = fmt.Sprintf("upload-%d", s.started)
	s.store.parts = map[int][]byte{}
	s.store.mu.Unlock()
	return Plan{UploadID: s.store.uploadID, PartSize: s.part}, nil
}

func (s *signer) Sign(ctx context.Context, uploadID string, numbers []int) (map[int]string, error) {
	s.mu.Lock()
	s.signed = append(s.signed, slices.Clone(numbers))
	fail := s.failSign
	if fail > 0 {
		s.failSign--
	}
	s.mu.Unlock()
	if fail > 0 {
		return nil, errors.New("gone")
	}
	urls := make(map[int]string, len(numbers))
	for _, number := range numbers {
		urls[number] = s.url + "/part?number=" + strconv.Itoa(number)
	}
	return urls, nil
}

func (s *signer) Complete(ctx context.Context, uploadID string, parts []Part) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.complete = slices.Clone(parts)
	s.store.mu.Lock()
	s.store.complete = true
	s.store.mu.Unlock()
	return nil
}

func (s *signer) Abort(ctx context.Context, uploadID string) error {
	s.store.mu.Lock()
	s.store.aborted = true
	s.store.mu.Unlock()
	return nil
}

func TestSingleRequest(t *testing.T) {
	server := newStore()
	fake := &signer{store: server, url: server.server(t).URL, single: true}
	file := memoryFile{name: "photo.jpg", data: []byte("0123456789")}

	var last Progress
	result, err := Run(context.Background(), file, fake, Options{
		OnProgress: func(p Progress) { last = p },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ETag != "whole-etag" {
		t.Fatalf("etag %q", result.ETag)
	}
	server.mu.Lock()
	got := string(server.parts[0])
	server.mu.Unlock()
	if got != "0123456789" {
		t.Fatalf("server has %q", got)
	}
	if last.Sent != 10 || last.Total != 10 || last.Done != 1 || last.Parts != 1 {
		t.Fatalf("progress %+v", last)
	}
}

func TestParts(t *testing.T) {
	server := newStore()
	fake := &signer{store: server, url: server.server(t).URL, part: 4}
	data := []byte("0123456789")
	file := memoryFile{name: "photo.jpg", data: data}
	var last Progress
	result, err := Run(context.Background(), file, fake, Options{
		OnProgress: func(p Progress) { last = p },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.UploadID != "upload-1" {
		t.Fatalf("upload %q", result.UploadID)
	}
	want := []Part{{1, "etag-1", 4}, {2, "etag-2", 4}, {3, "etag-3", 2}}
	if !slices.Equal(result.Parts, want) {
		t.Fatalf("parts %+v", result.Parts)
	}
	if !slices.Equal(fake.complete, want) {
		t.Fatalf("complete %+v", fake.complete)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if !server.complete {
		t.Fatal("the server did not join the parts")
	}
	joined := append(append(append([]byte{}, server.parts[1]...), server.parts[2]...), server.parts[3]...)
	if string(joined) != string(data) {
		t.Fatalf("server has %q", joined)
	}
	if last.Sent != 10 || last.Done != 3 || last.Parts != 3 {
		t.Fatalf("progress %+v", last)
	}
}

func TestResume(t *testing.T) {
	server := newStore()
	server.fail[2] = 100
	fake := &signer{store: server, url: server.server(t).URL, part: 4}
	file := memoryFile{name: "photo.jpg", data: []byte("0123456789")}
	memory := NewMemory()

	_, err := Run(context.Background(), file, fake, Options{
		Store:       memory,
		Retry:       Retry{Attempts: 1},
		Concurrency: 1,
	})
	if err == nil {
		t.Fatal("a part that always fails did not fail")
	}
	state, ok := memory.Load("photo.jpg:10")
	if !ok {
		t.Fatal("the upload was not kept")
	}
	kept := map[int]bool{}
	for _, part := range state.Parts {
		kept[part.Number] = true
	}
	missing := []int{}
	for number := 1; number <= 3; number++ {
		if !kept[number] {
			missing = append(missing, number)
		}
	}
	if slices.Equal(missing, []int{}) {
		t.Fatalf("a failed part was kept: %+v", state.Parts)
	}

	server.mu.Lock()
	server.fail = map[int]int{}
	server.mu.Unlock()
	fake.mu.Lock()
	fake.signed = nil
	fake.mu.Unlock()

	result, err := Run(context.Background(), file, fake, Options{Store: memory, PartSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Parts) != 3 {
		t.Fatalf("result %+v", result.Parts)
	}
	fake.mu.Lock()
	signed := fake.signed
	fake.mu.Unlock()
	if len(signed) != 1 || !slices.Equal(signed[0], missing) {
		t.Fatalf("signed %v, missing %v", signed, missing)
	}
	if _, ok := memory.Load("photo.jpg:10"); ok {
		t.Fatal("a finished upload was kept")
	}
}

func TestStaleSessionStartsAgain(t *testing.T) {
	server := newStore()
	fake := &signer{store: server, url: server.server(t).URL, part: 4}
	file := memoryFile{name: "photo.jpg", data: []byte("0123456789")}
	memory := NewMemory()
	memory.Save("photo.jpg:10", State{UploadID: "forgotten", PartSize: 4, Size: 10, Parts: []Part{{Number: 1, Size: 4}}})

	fake.failSign = 1
	result, err := Run(context.Background(), file, fake, Options{Store: memory})
	if err != nil {
		t.Fatal(err)
	}
	if result.UploadID != "upload-1" {
		t.Fatalf("upload %q", result.UploadID)
	}
	if len(result.Parts) != 3 {
		t.Fatalf("parts %+v", result.Parts)
	}
	if fake.started != 1 {
		t.Fatalf("started %d times", fake.started)
	}
}

func TestRetrySendsAgain(t *testing.T) {
	server := newStore()
	server.fail[1] = 1
	fake := &signer{store: server, url: server.server(t).URL, part: 4}
	file := memoryFile{name: "photo.jpg", data: []byte("0123456789")}
	if _, err := Run(context.Background(), file, fake, Options{
		Retry: Retry{Attempts: 3, Base: time.Millisecond, Max: time.Millisecond},
	}); err != nil {
		t.Fatal(err)
	}
	if got := server.requests.Load(); got != 4 {
		t.Fatalf("requests %d", got)
	}
}

func TestClientErrorIsNotRetried(t *testing.T) {
	server := newStore()
	fake := &signer{store: server, url: server.server(t).URL, part: 4}
	// A 503 is worth retrying; a 403 is not.
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.requests.Add(1)
		w.WriteHeader(http.StatusForbidden)
	}))
	defer refusing.Close()
	fake.url = refusing.URL
	file := memoryFile{name: "photo.jpg", data: []byte("0123")}
	if _, err := Run(context.Background(), file, fake, Options{PartSize: 4}); err == nil {
		t.Fatal("a forbidden part did not fail")
	}
	if got := server.requests.Load(); got != 1 {
		t.Fatalf("requests %d", got)
	}
}

func TestConcurrency(t *testing.T) {
	server := newStore()
	fake := &signer{store: server, url: server.server(t).URL, part: 1}
	file := memoryFile{name: "photo.jpg", data: []byte("0123456789")}
	if _, err := Run(context.Background(), file, fake, Options{Concurrency: 2}); err != nil {
		t.Fatal(err)
	}
	server.mu.Lock()
	most := server.most
	server.mu.Unlock()
	if most > 2 {
		t.Fatalf("%d parts went out at once", most)
	}
}

func TestCancel(t *testing.T) {
	server := newStore()
	fake := &signer{store: server, url: server.server(t).URL, part: 4}
	file := memoryFile{name: "photo.jpg", data: []byte("0123456789")}
	memory := NewMemory()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Run(ctx, file, fake, Options{Store: memory})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v", err)
	}
	server.mu.Lock()
	complete := server.complete
	server.mu.Unlock()
	if complete {
		t.Fatal("a canceled upload was completed")
	}
}

func TestSignsInBatches(t *testing.T) {
	called := 0
	numbers := make([]int, 150)
	for i := range numbers {
		numbers[i] = i + 1
	}
	fake := batchSigner{count: &called}
	urls, err := Options{}.sign(context.Background(), fake, "upload", numbers)
	if err != nil {
		t.Fatal(err)
	}
	if called != 3 {
		t.Fatalf("asked %d times", called)
	}
	if len(urls) != 150 {
		t.Fatalf("urls %d", len(urls))
	}
}

type batchSigner struct{ count *int }

func (b batchSigner) Start(context.Context, Info) (Plan, error) { return Plan{}, nil }
func (b batchSigner) Sign(ctx context.Context, id string, numbers []int) (map[int]string, error) {
	*b.count = *b.count + 1
	urls := make(map[int]string, len(numbers))
	for _, number := range numbers {
		urls[number] = "http://example.invalid/" + strconv.Itoa(number)
	}
	return urls, nil
}
func (b batchSigner) Complete(context.Context, string, []Part) error { return nil }
func (b batchSigner) Abort(context.Context, string) error            { return nil }

func TestAbort(t *testing.T) {
	memory := NewMemory()
	memory.Save("key", State{UploadID: "upload"})
	fake := &signer{store: newStore()}
	if err := Abort(context.Background(), fake, memory, "key"); err != nil {
		t.Fatal(err)
	}
	if !fake.store.aborted {
		t.Fatal("the upload was not aborted")
	}
	if _, ok := memory.Load("key"); ok {
		t.Fatal("an aborted upload was kept")
	}
}

func TestPartSize(t *testing.T) {
	if got := PartSize(10); got != minPart {
		t.Fatalf("size %d", got)
	}
	if got := PartSize(100 << 30); got < 10<<20 {
		t.Fatalf("size %d", got)
	}
	if got := PartSize(1 << 40); got > maxPart {
		t.Fatalf("size %d", got)
	}
	if got := PartSize(1 << 62); got != maxPart {
		t.Fatalf("size %d", got)
	}
	if count := partCount(100<<30, PartSize(100<<30)); count > maxParts {
		t.Fatalf("%d parts", count)
	}
}
