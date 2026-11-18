//go:build !js

package auth

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Callback waits for the browser to come back from the provider: it listens on
// the loopback address the flow redirects to, and answers the page that says
// the sign-in goes on in the app.
//
// Close it when the sign-in ends, whichever way it ends.
type Callback struct {
	listener net.Listener
	server   *http.Server
	answer   chan url.Values
	base     string
}

// Listen listens for the browser on address, as "127.0.0.1:0" for the first
// port the system gives: a loopback address only the machine reaches.
func Listen(address string) (*Callback, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if host != "127.0.0.1" && host != "::1" && host != "localhost" {
		return nil, errors.New("auth: the callback must listen on the loopback address")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	callback := &Callback{
		listener: listener,
		answer:   make(chan url.Values, 1),
		base:     "http://" + listener.Addr().String(),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, request *http.Request) {
		select {
		case callback.answer <- request.URL.Query():
		default:
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Signed in</title><p>You can close this window and go back to the app.`))
	})
	callback.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = callback.server.Serve(listener) }()
	return callback, nil
}

// URL is where the provider sends the browser back.
func (c *Callback) URL() string { return c.base + "/callback" }

// Wait waits for the browser to come back, and returns what the provider sent.
func (c *Callback) Wait(ctx context.Context) (url.Values, error) {
	select {
	case answer := <-c.answer:
		return answer, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close stops listening.
func (c *Callback) Close() error { return c.server.Close() }
