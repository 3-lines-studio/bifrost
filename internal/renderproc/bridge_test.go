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
	"testing"
)

func bridgeForHandler(t *testing.T, handler http.Handler) *Bridge {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	address := server.Listener.Addr().String()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", address)
	}}
	return &Bridge{client: &http.Client{Transport: transport}}
}

func TestBridgeRenderDecodesOrderedFrames(t *testing.T) {
	bridge := bridgeForHandler(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var payload struct {
			Entry string          `json:"entry"`
			Props json.RawMessage `json:"props"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Entry != "entry.js" || string(payload.Props) != `{"name":"Don"}` {
			t.Errorf("payload = %+v", payload)
		}
		writeFrameToResponse(w, frameHead, "<title>Hi</title>")
		writeFrameToResponse(w, frameBody, "<main>Hi</main>")
		writeFrameToResponse(w, frameDone, "")
	}))
	sink := &testSink{}
	if err := bridge.Render(context.Background(), "entry.js", json.RawMessage(`{"name":"Don"}`), sink); err != nil {
		t.Fatal(err)
	}
	if sink.head != "<title>Hi</title>" || sink.body != "<main>Hi</main>" {
		t.Fatalf("sink = %+v", sink)
	}
}

func TestBridgeHealthyChecksStatus(t *testing.T) {
	healthy := bridgeForHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if err := healthy.Healthy(context.Background()); err != nil {
		t.Fatal(err)
	}
	unhealthy := bridgeForHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	if err := unhealthy.Healthy(context.Background()); err == nil {
		t.Fatal("unhealthy bridge reported ready")
	}
}

func TestBridgeRenderClassifiesRemoteError(t *testing.T) {
	bridge := bridgeForHandler(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeFrameToResponse(w, frameError, "bad component")
	}))
	err := bridge.Render(context.Background(), "entry.js", nil, &testSink{})
	var remote *RemoteError
	if !errors.As(err, &remote) {
		t.Fatalf("error = %T %v, want RemoteError", err, err)
	}
}

func writeFrameToResponse(w http.ResponseWriter, kind byte, payload string) {
	var header [5]byte
	header[0] = kind
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))
	_, _ = w.Write(header[:])
	_, _ = w.Write([]byte(payload))
}
