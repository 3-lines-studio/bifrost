package bifrost

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type redirectError struct {
	url    string
	status int
}

func (e redirectError) Error() string { return fmt.Sprintf("redirect to %s", e.url) }

// Redirect returns a loader error that sends an HTTP redirect.
func Redirect(url string, statuses ...int) error {
	if url == "" {
		return errors.New("bifrost: redirect URL is empty")
	}
	if strings.ContainsAny(url, "\r\n") {
		return errors.New("bifrost: redirect URL contains a line break")
	}
	status := http.StatusFound
	if len(statuses) > 1 {
		return errors.New("bifrost: redirect accepts at most one status")
	}
	if len(statuses) == 1 {
		status = statuses[0]
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
func NotFound(causes ...error) error {
	if len(causes) > 1 {
		return errors.New("bifrost: not found accepts at most one cause")
	}
	if len(causes) == 0 {
		return notFoundError{}
	}
	return notFoundError{cause: causes[0]}
}

type statusError struct {
	status int
	cause  error
}

func (e statusError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return http.StatusText(e.status)
}

func (e statusError) Unwrap() error { return e.cause }

func Status(status int, cause error) error {
	if status < 400 || status > 599 {
		return fmt.Errorf("bifrost: invalid error status %d", status)
	}
	return statusError{status: status, cause: cause}
}

func IsRedirect(err error) bool {
	var redirect redirectError
	return errors.As(err, &redirect)
}

func ErrorStatus(err error) (int, bool) {
	var missing notFoundError
	if errors.As(err, &missing) {
		return http.StatusNotFound, true
	}
	var status statusError
	if errors.As(err, &status) {
		return status.status, true
	}
	return 0, false
}

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
	var status statusError
	if errors.As(err, &status) {
		http.Error(w, http.StatusText(status.status), status.status)
		return
	}
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
