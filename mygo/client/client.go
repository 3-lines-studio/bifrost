// Package client calls an HTTP server from the platforms the app runs on.
//
// In a browser it uses fetch, so the build carries no HTTP stack of its own
// and the browser does TLS; elsewhere it uses net/http. Both answer the same
// values, so a screen calls the server the same way on every target.
//
// A call blocks until the server answers. In a browser the answer arrives on
// the goroutine the app draws on, so a call made there waits forever, and Do
// answers ErrUIThread instead: call from a goroutine and apply what it answered
// in MyGo's Window.Update.
//
//	go func() {
//		notes, err := client.GetJSON[[]Note](ctx, "/api/notes", session.Header())
//		win.Update(func() { app.apply(notes, err) })
//	}()
package client

import (
	"context"
	"encoding/json"
	"fmt"
)

// Request is a call to the server. Header holds the request headers, as
// "Authorization": "Bearer …"; Body is the request body, nil for none.
type Request struct {
	Method string
	URL    string
	Header map[string]string
	Body   []byte
}

// Response is what the server answered.
type Response struct {
	Status int
	Header map[string]string
	Body   []byte
}

// Do sends request and reads the whole answer.
func Do(ctx context.Context, request Request) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if request.Method == "" {
		request.Method = "GET"
	}
	return send(ctx, request)
}

// Get returns the body of a GET request that answered 2xx.
func Get(ctx context.Context, url string, header map[string]string) ([]byte, error) {
	response, err := Do(ctx, Request{Method: "GET", URL: url, Header: header})
	if err != nil {
		return nil, err
	}
	if response.Status < 200 || response.Status > 299 {
		return nil, statusError("GET", url, response.Status)
	}
	return response.Body, nil
}

// GetJSON returns the value a GET request answered with.
func GetJSON[T any](ctx context.Context, url string, header map[string]string) (T, error) {
	var value T
	body, err := Get(ctx, url, header)
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(body, &value)
	return value, err
}

// SendJSON sends input as JSON and returns the JSON value of the answer.
func SendJSON[T any](ctx context.Context, method, url string, header map[string]string, input any) (T, error) {
	var value T
	body, err := json.Marshal(input)
	if err != nil {
		return value, err
	}
	headers := make(map[string]string, len(header)+1)
	for name, value := range header {
		headers[name] = value
	}
	headers["Content-Type"] = "application/json"
	response, err := Do(ctx, Request{Method: method, URL: url, Header: headers, Body: body})
	if err != nil {
		return value, err
	}
	if response.Status < 200 || response.Status > 299 {
		return value, statusError(method, url, response.Status)
	}
	err = json.Unmarshal(response.Body, &value)
	return value, err
}

func statusError(method, url string, status int) error {
	return fmt.Errorf("%s %s: %d", method, url, status)
}
