package uploadproto_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/3-lines-studio/bifrost/mygo/client"
	"github.com/3-lines-studio/bifrost/mygo/demo/uploadproto"
	"github.com/3-lines-studio/bifrost/mygo/file"
	"github.com/3-lines-studio/bifrost/mygo/upload"
)

// server mounts the endpoints of an upload, as the App Router does.
func server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/upload/start", uploadproto.Start)
	mux.HandleFunc("POST /api/upload/sign", uploadproto.Sign)
	mux.HandleFunc("PUT /api/upload/part", uploadproto.Part)
	mux.HandleFunc("POST /api/upload/complete", uploadproto.Complete)
	mux.HandleFunc("POST /api/upload/abort", uploadproto.Abort)
	mux.HandleFunc("GET /api/media/{id}", uploadproto.Media)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// call is the app's call, without a session.
type call struct{ base string }

func (c call) Call(ctx context.Context, method, path string, input, output any) error {
	header := map[string]string{}
	var body []byte
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body, header["Content-Type"] = encoded, "application/json"
	}
	response, err := client.Do(ctx, client.Request{Method: method, URL: c.base + path, Header: header, Body: body})
	if err != nil {
		return err
	}
	if response.Status < 200 || response.Status > 299 {
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(response.Body, &failure) == nil && failure.Error != "" {
			return fmt.Errorf("%s", failure.Error)
		}
		return fmt.Errorf("%s %s answered %d", method, path, response.Status)
	}
	if output == nil {
		return nil
	}
	return json.Unmarshal(response.Body, output)
}

// picture writes a file of size bytes, as one the person picked.
func picture(t *testing.T, size int) (string, []byte) {
	t.Helper()
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i*31 + i/7)
	}
	path := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data
}

func TestUploadRoundTrip(t *testing.T) {
	server := server(t)
	path, data := picture(t, 2*1024*1024+512*1024)
	picture, err := file.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer picture.Close()

	var last upload.Progress
	signer := uploadproto.NewSigner(server.URL, call{base: server.URL})
	result, err := upload.Run(context.Background(), picture, signer, upload.Options{
		OnProgress: func(p upload.Progress) { last = p },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Parts) != 3 {
		t.Fatalf("%d parts", len(result.Parts))
	}
	if last.Sent != int64(len(data)) || last.Done != 3 || last.Parts != 3 {
		t.Fatalf("progress %+v", last)
	}

	response, err := http.Get(server.URL + "/api/media/" + result.UploadID)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("media status %d", response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("media type %q", got)
	}
	joined, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(joined, data) {
		t.Fatalf("the server joined %d bytes of %d", len(joined), len(data))
	}
}

func TestCompleteRejectsAPartTheServerHasNot(t *testing.T) {
	server := server(t)
	path, _ := picture(t, 100)
	picture, err := file.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer picture.Close()
	app := call{base: server.URL}
	signer := uploadproto.NewSigner(server.URL, app)

	var started upload.Plan
	started, err = signer.Start(context.Background(), upload.Info{Name: picture.Name(), Size: picture.Size(), Type: picture.ContentType()})
	if err != nil {
		t.Fatal(err)
	}
	err = signer.Complete(context.Background(), started.UploadID, []upload.Part{{Number: 1, ETag: "not the one", Size: 100}})
	if err == nil {
		t.Fatal("the server joined a part it does not have")
	}
}

func TestAbortForgetsTheUpload(t *testing.T) {
	server := server(t)
	path, _ := picture(t, 100)
	picture, err := file.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer picture.Close()
	signer := uploadproto.NewSigner(server.URL, call{base: server.URL})
	started, err := signer.Start(context.Background(), upload.Info{Name: picture.Name(), Size: picture.Size(), Type: picture.ContentType()})
	if err != nil {
		t.Fatal(err)
	}
	if err := signer.Abort(context.Background(), started.UploadID); err != nil {
		t.Fatal(err)
	}
	_, err = signer.Sign(context.Background(), started.UploadID, []int{1})
	if err == nil {
		t.Fatal("an aborted upload was signed")
	}
}
