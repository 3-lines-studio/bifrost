package bifrost

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

// dataMediaType asks a server route for the props its loader produced instead
// of a rendered document. A native client renders the same view itself, so it
// needs the loader's data and nothing the renderer would add.
const dataMediaType = "application/vnd.bifrost.data+json"

type dataPage struct {
	Pattern  string                      `json:"pattern"`
	Props    json.RawMessage             `json:"props"`
	Document protocol.DocumentAttributes `json:"document"`
	Status   int                         `json:"status"`
	// A page that throws while it renders falls into the nearest error.tsx, and
	// the view the generator wrote picks which one out of `__bifrostErrorLevel`.
	// The server walks that ladder itself; a client that renders the same view
	// has to walk it too, so it has to know how deep it goes.
	ErrorFallbacks int `json:"errorFallbacks"`
}

type dataError struct {
	Error  string `json:"error"`
	Status int    `json:"status"`
}

// An Accept header is a list of media types with optional parameters, and the
// data type is one of them: a client that asks for the props and also says it
// takes anything is asking for the props, and the server that reads the header
// as one string would have missed it.
func dataRequested(request *http.Request) bool {
	if request.Method != http.MethodGet {
		return false
	}
	for _, part := range strings.Split(request.Header.Get("Accept"), ",") {
		media := strings.TrimSpace(part)
		if before, _, found := strings.Cut(media, ";"); found {
			media = strings.TrimSpace(before)
		}
		if media == dataMediaType {
			return true
		}
	}
	return false
}

func writeDataPage(w http.ResponseWriter, pattern string, props json.RawMessage, document Document, status, errorFallbacks int) error {
	w.Header().Set("Content-Type", dataMediaType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(dataPage{
		Pattern:        pattern,
		Props:          props,
		Document:       protocolDocument(document),
		Status:         status,
		ErrorFallbacks: errorFallbacks,
	})
}

func serveDataError(w http.ResponseWriter, request *http.Request, err error) {
	status := http.StatusInternalServerError
	var redirect redirectError
	if errors.As(err, &redirect) {
		w.Header().Set("Location", redirect.url)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(redirect.status)
		return
	}
	var missing notFoundError
	if errors.As(err, &missing) {
		status = http.StatusNotFound
	}
	var failure statusError
	if errors.As(err, &failure) {
		status = failure.status
	}
	if errors.Is(err, ErrRendererBusy) {
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", dataMediaType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(dataError{Error: http.StatusText(status), Status: status})
}
