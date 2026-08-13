//go:build !windows

package renderproc

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testSink struct {
	head string
	body string
}

func (s *testSink) Head(value []byte) error { s.head = string(value); return nil }
func (s *testSink) Body(value []byte) error { s.body += string(value); return nil }

func processForHandler(t *testing.T, handler http.Handler) *Process {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	address := server.Listener.Addr().String()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", address)
	}}
	return &Process{client: &http.Client{Transport: transport}}
}

func writeTestFrame(t *testing.T, w http.ResponseWriter, kind byte, payload string) {
	t.Helper()
	var header [5]byte
	header[0] = kind
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		t.Error(err)
	}
	if _, err := w.Write([]byte(payload)); err != nil {
		t.Error(err)
	}
}

func TestRenderDecodesOrderedFrames(t *testing.T) {
	process := processForHandler(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload renderPayload
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Entry != "entry.js" || string(payload.Props) != `{"name":"Don"}` {
			t.Errorf("payload = %+v", payload)
		}
		writeTestFrame(t, w, 1, "<title>Hi</title>")
		writeTestFrame(t, w, 2, "<main>")
		writeTestFrame(t, w, 2, "Hi</main>")
		writeTestFrame(t, w, 3, "")
	}))
	sink := &testSink{}
	if err := process.Render(context.Background(), "entry.js", json.RawMessage(`{"name":"Don"}`), sink); err != nil {
		t.Fatal(err)
	}
	if sink.head != "<title>Hi</title>" || sink.body != "<main>Hi</main>" {
		t.Fatalf("sink = %+v", sink)
	}
}

func TestRuntimeSourcesPropagateCancellation(t *testing.T) {
	for name, source := range map[string]string{"production": RuntimeSource, "development": DevRuntimeSource} {
		t.Run(name, func(t *testing.T) {
			for _, required := range []string{
				"module.render(input.props ?? {}, request.signal)",
				"reader?.cancel(request.signal.reason)",
				"cancel(reason)",
			} {
				if !strings.Contains(source, required) {
					t.Fatalf("runtime source does not contain %q", required)
				}
			}
		})
	}
}

func TestHealthyChecksStatus(t *testing.T) {
	healthy := processForHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if err := healthy.Healthy(context.Background()); err != nil {
		t.Fatal(err)
	}

	unhealthy := processForHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	if err := unhealthy.Healthy(context.Background()); err == nil {
		t.Fatal("unhealthy renderer reported ready")
	}
}

func TestRenderClassifiesRemoteError(t *testing.T) {
	process := processForHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTestFrame(t, w, 4, "bad component")
	}))
	err := process.Render(context.Background(), "entry.js", json.RawMessage(`{}`), &testSink{})
	var remote *RemoteError
	if !errors.As(err, &remote) {
		t.Fatalf("error = %T %v, want RemoteError", err, err)
	}
}
