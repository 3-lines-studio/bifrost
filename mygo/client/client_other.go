//go:build !js

package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

func send(ctx context.Context, request Request) (Response, error) {
	body, length, err := request.body()
	if err != nil {
		return Response{}, err
	}
	outgoing, err := http.NewRequestWithContext(ctx, request.Method, request.URL, body)
	if err != nil {
		return Response{}, err
	}
	if body != nil {
		outgoing.ContentLength = length
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

// body returns what the request sends and how many bytes it is: the blob where
// it is, or its bytes, both counted as they go for OnUpload.
func (r Request) body() (io.Reader, int64, error) {
	var reader io.Reader
	var length int64
	switch {
	case r.Blob.present():
		blob, size := r.Blob.open()
		reader, length = blob, size
	case len(r.Body) > 0:
		reader, length = bytes.NewReader(r.Body), int64(len(r.Body))
	default:
		return nil, 0, nil
	}
	if r.OnUpload == nil {
		return reader, length, nil
	}
	return &countingReader{reader: reader, on: r.OnUpload}, length, nil
}

// countingReader reports how many bytes a request has read from it.
type countingReader struct {
	reader io.Reader
	sent   int64
	on     func(int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	if n > 0 {
		c.sent += int64(n)
		c.on(c.sent)
	}
	return n, err
}

// Copy sends request and writes the body of the answer to w as it comes,
// reporting the bytes received so far as they arrive. It returns how many
// bytes it wrote.
func Copy(ctx context.Context, request Request, w io.Writer, onProgress func(received int64)) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if request.Method == "" {
		request.Method = "GET"
	}
	outgoing, err := http.NewRequestWithContext(ctx, request.Method, request.URL, nil)
	if err != nil {
		return 0, err
	}
	for name, value := range request.Header {
		outgoing.Header.Set(name, value)
	}
	answer, err := http.DefaultClient.Do(outgoing)
	if err != nil {
		return 0, err
	}
	defer answer.Body.Close()
	if answer.StatusCode < 200 || answer.StatusCode > 299 {
		return 0, statusError(request.Method, request.URL, answer.StatusCode)
	}
	if onProgress == nil {
		return io.Copy(w, answer.Body)
	}
	counted := &countingWriter{writer: w, on: onProgress}
	return io.Copy(counted, answer.Body)
}

// countingWriter reports how many bytes a download has written.
type countingWriter struct {
	writer   io.Writer
	received int64
	on       func(int64)
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.writer.Write(p)
	if n > 0 {
		c.received += int64(n)
		c.on(c.received)
	}
	return n, err
}
