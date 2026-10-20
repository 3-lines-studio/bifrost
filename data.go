package bifrost

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/3-lines-studio/bifrost/internal/protocol"
)

const dataMediaType = "application/vnd.bifrost.data+json"

type dataPage struct {
	Pattern        string                      `json:"pattern"`
	Props          json.RawMessage             `json:"props"`
	Document       protocol.DocumentAttributes `json:"document"`
	Status         int                         `json:"status"`
	ErrorFallbacks int                         `json:"errorFallbacks"`
}

type dataError struct {
	Error  string `json:"error"`
	Status int    `json:"status"`
}

func dataRequested(request *http.Request) bool {
	return request.Method == http.MethodGet && request.Header.Get("Accept") == dataMediaType
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
