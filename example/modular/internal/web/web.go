package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Transport sentinels shared by every module. Modules alias these so the single
// WriteError mapper below classifies their errors by errors.Is.
var (
	ErrBadRequest   = errors.New("bad request")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrValidation   = errors.New("validation")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
)

// WriteJSON encodes v as JSON with the given status. Every module uses this one
// writer, so successful responses are uniform.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError maps a sentinel error to an HTTP status and writes a JSON error
// body. Unknown errors fall back to 500. Every module uses this one mapper.
func WriteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrBadRequest):
		status = http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, ErrValidation):
		status = http.StatusUnprocessableEntity
	}
	WriteJSON(w, status, map[string]string{"error": err.Error()})
}

// DecodeJSON decodes a request body into v, bounded to 1 MiB.
func DecodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

// DecodeTask decodes an asynq task payload into v.
func DecodeTask(payload []byte, v any) error {
	return json.Unmarshal(payload, v)
}
