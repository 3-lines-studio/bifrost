package bifrost

import (
	"errors"
	"fmt"
	"net/http"
)

type redirectError struct {
	url    string
	status int
}

func (e redirectError) Error() string { return fmt.Sprintf("redirect to %s", e.url) }

// Redirect returns a loader error that sends an HTTP redirect.
func Redirect(url string, status int) error {
	if url == "" {
		return errors.New("bifrost: redirect URL is empty")
	}
	if status == 0 {
		status = http.StatusFound
	}
	if status < 300 || status > 399 {
		return fmt.Errorf("bifrost: invalid redirect status %d", status)
	}
	return redirectError{url: url, status: status}
}

type notFoundError struct{ cause error }

func (e notFoundError) Error() string {
	if e.cause == nil {
		return "not found"
	}
	return e.cause.Error()
}

func (e notFoundError) Unwrap() error { return e.cause }

// NotFound returns a loader error that sends a 404 response.
func NotFound(cause error) error { return notFoundError{cause: cause} }

func serveDefaultError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrRendererBusy) {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	var redirect redirectError
	if errors.As(err, &redirect) {
		http.Redirect(w, r, redirect.url, redirect.status)
		return
	}
	var missing notFoundError
	if errors.As(err, &missing) {
		http.NotFound(w, r)
		return
	}
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
