// Package uploadproto is the two ends of an upload, as S3 takes one: the server
// that signs the parts and joins them, and the signer the app hands to
// mygo/upload. The demo keeps the parts in memory; a store of objects keeps
// them in a bucket, and the protocol is the same.
package uploadproto

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/3-lines-studio/bifrost/mygo/demo/store"
	"github.com/3-lines-studio/bifrost/mygo/upload"
)

// MaxPart is the largest part the server reads; the store checks the exact
// size of each one.
const MaxPart = store.PartSize

// Start begins an upload of the file the body describes.
func Start(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		Type string `json:"type"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	started, err := store.StartUpload(input.Name, input.Type, input.Size)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	answer(w, http.StatusOK, map[string]any{"uploadId": started.ID, "partSize": started.PartSize})
}

// Sign answers the URL of each part the client asks for.
func Sign(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UploadID string `json:"uploadId"`
		Numbers  []int  `json:"numbers"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	started, ok := store.UploadByID(input.UploadID)
	if !ok {
		fail(w, http.StatusNotFound, store.ErrNoUpload)
		return
	}
	count := int((started.Size + started.PartSize - 1) / started.PartSize)
	urls := make(map[string]string, len(input.Numbers))
	for _, number := range input.Numbers {
		if number < 1 || number > count {
			fail(w, http.StatusBadRequest, fmt.Errorf("no part %d of %d", number, count))
			return
		}
		urls[strconv.Itoa(number)] = "/api/upload/part?" + url.Values{
			"upload": {input.UploadID},
			"number": {strconv.Itoa(number)},
		}.Encode()
	}
	answer(w, http.StatusOK, map[string]any{"urls": urls})
}

// Part keeps the bytes of one part and answers its ETag.
func Part(w http.ResponseWriter, r *http.Request) {
	number, err := strconv.Atoi(r.URL.Query().Get("number"))
	if err != nil {
		fail(w, http.StatusBadRequest, errors.New("a part needs a number"))
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, MaxPart+1))
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	part, err := store.PutPart(r.URL.Query().Get("upload"), number, data)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, store.ErrNoUpload) {
			status = http.StatusNotFound
		}
		fail(w, status, err)
		return
	}
	w.Header().Set("ETag", `"`+part.ETag+`"`)
	w.WriteHeader(http.StatusNoContent)
}

// Complete joins the parts the client sent.
func Complete(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UploadID string       `json:"uploadId"`
		Parts    []store.Part `json:"parts"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	media, err := store.CompleteUpload(input.UploadID, input.Parts)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, store.ErrNoUpload) {
			status = http.StatusNotFound
		}
		fail(w, status, err)
		return
	}
	answer(w, http.StatusOK, map[string]any{"url": "/api/media/" + media.ID, "name": media.Name, "size": media.Size})
}

// Abort drops an upload nobody finishes.
func Abort(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UploadID string `json:"uploadId"`
	}
	if err := decode(r, &input); err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	store.AbortUpload(input.UploadID)
	w.WriteHeader(http.StatusNoContent)
}

// Media answers the file of a joined upload, at the path its share link names.
func Media(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	media, ok := store.MediaByID(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", media.Type)
	w.Header().Set("Content-Length", strconv.Itoa(len(media.Data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Disposition", `inline; filename="`+media.Name+`"`)
	_, _ = w.Write(media.Data)
}

func decode(r *http.Request, value any) error {
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(value); err != nil {
		return errors.New("the body is not JSON")
	}
	return nil
}

func answer(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, status int, err error) {
	answer(w, status, map[string]string{"error": err.Error()})
}

// Client is how the signer reaches the server: the app's own call, which signs
// the request in, and a test's.
type Client interface {
	Call(ctx context.Context, method, path string, input, output any) error
}

// NewSigner returns the upload.Signer of the app: base is the address of the
// server, "" in a browser, where the paths go as they are.
func NewSigner(base string, client Client) upload.Signer {
	return &signer{base: base, client: client}
}

type signer struct {
	base   string
	client Client
}

func (s *signer) Start(ctx context.Context, file upload.Info) (upload.Plan, error) {
	var answer struct {
		UploadID string `json:"uploadId"`
		PartSize int64  `json:"partSize"`
	}
	input := map[string]any{"name": file.Name, "size": file.Size, "type": file.Type}
	if err := s.client.Call(ctx, "POST", "/api/upload/start", input, &answer); err != nil {
		return upload.Plan{}, err
	}
	return upload.Plan{UploadID: answer.UploadID, PartSize: answer.PartSize}, nil
}

func (s *signer) Sign(ctx context.Context, uploadID string, numbers []int) (map[int]string, error) {
	var answer struct {
		URLs map[string]string `json:"urls"`
	}
	input := map[string]any{"uploadId": uploadID, "numbers": numbers}
	if err := s.client.Call(ctx, "POST", "/api/upload/sign", input, &answer); err != nil {
		return nil, err
	}
	urls := make(map[int]string, len(answer.URLs))
	for number, path := range answer.URLs {
		value, err := strconv.Atoi(number)
		if err != nil {
			return nil, fmt.Errorf("upload: the server signed %q", number)
		}
		urls[value] = s.base + path
	}
	return urls, nil
}

func (s *signer) Complete(ctx context.Context, uploadID string, parts []upload.Part) error {
	held := make([]store.Part, 0, len(parts))
	for _, part := range parts {
		held = append(held, store.Part{Number: part.Number, ETag: part.ETag, Size: part.Size})
	}
	input := map[string]any{"uploadId": uploadID, "parts": held}
	return s.client.Call(ctx, "POST", "/api/upload/complete", input, nil)
}

func (s *signer) Abort(ctx context.Context, uploadID string) error {
	return s.client.Call(ctx, "POST", "/api/upload/abort", map[string]string{"uploadId": uploadID}, nil)
}
