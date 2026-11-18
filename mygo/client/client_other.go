//go:build !js

package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

func send(ctx context.Context, request Request) (Response, error) {
	var body io.Reader
	if len(request.Body) > 0 {
		body = bytes.NewReader(request.Body)
	}
	outgoing, err := http.NewRequestWithContext(ctx, request.Method, request.URL, body)
	if err != nil {
		return Response{}, err
	}
	for name, value := range request.Header {
		outgoing.Header.Set(name, value)
	}
	answer, err := http.DefaultClient.Do(outgoing)
	if err != nil {
		return Response{}, err
	}
	defer answer.Body.Close()
	data, err := io.ReadAll(answer.Body)
	if err != nil {
		return Response{}, err
	}
	header := make(map[string]string, len(answer.Header))
	for name := range answer.Header {
		header[name] = answer.Header.Get(name)
	}
	return Response{Status: answer.StatusCode, Header: header, Body: data}, nil
}
