//go:build js && wasm

package client

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"sync"
	"syscall/js"
)

// ErrUIThread is what a request made where the app draws answers. The browser
// hands the answer to the goroutine that draws, so one that waits for it there
// waits forever; send the request from a goroutine and apply what it answered
// in Window.Update.
var ErrUIThread = errors.New("client: a request made on the thread that draws would wait forever; send it from a goroutine")

type outcome struct {
	response Response
	err      error
}

// send makes the request with fetch, so the browser does the network and
// TLS, and the build carries no HTTP stack.
func send(ctx context.Context, request Request) (Response, error) {
	if draws() {
		return Response{}, ErrUIThread
	}
	options := js.Global().Get("Object").New()
	options.Set("method", request.Method)
	if len(request.Header) > 0 {
		header := js.Global().Get("Object").New()
		for name, value := range request.Header {
			header.Set(name, value)
		}
		options.Set("headers", header)
	}
	if len(request.Body) > 0 {
		body := js.Global().Get("Uint8Array").New(len(request.Body))
		js.CopyBytesToJS(body, request.Body)
		options.Set("body", body)
	}
	controller := js.Global().Get("AbortController").New()
	options.Set("signal", controller.Get("signal"))

	done := make(chan outcome, 1)
	var onResponse, onBody, onFailure js.Func
	var once sync.Once
	release := func() {
		once.Do(func() {
			onResponse.Release()
			onBody.Release()
			onFailure.Release()
		})
	}
	finish := func(value outcome) {
		release()
		done <- value
	}

	var status int
	var header map[string]string
	onBody = js.FuncOf(func(_ js.Value, args []js.Value) any {
		body := make([]byte, 0)
		if len(args) > 0 && args[0].Type() == js.TypeObject {
			array := js.Global().Get("Uint8Array").New(args[0])
			body = make([]byte, array.Length())
			js.CopyBytesToGo(body, array)
		}
		finish(outcome{response: Response{Status: status, Header: header, Body: body}})
		return nil
	})
	onFailure = js.FuncOf(func(_ js.Value, args []js.Value) any {
		err := ctx.Err()
		if err == nil {
			err = errors.New("client: the request failed")
		}
		finish(outcome{err: err})
		return nil
	})
	onResponse = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 || args[0].Type() != js.TypeObject {
			finish(outcome{err: errors.New("client: no response")})
			return nil
		}
		response := args[0]
		status = response.Get("status").Int()
		header = readHeader(response.Get("headers"))
		response.Call("arrayBuffer").Call("then", onBody, onFailure)
		return nil
	})

	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			controller.Call("abort")
		case <-finished:
		}
	}()
	js.Global().Call("fetch", request.URL, options).Call("then", onResponse, onFailure)
	result := <-done
	close(finished)
	return result.response, result.err
}

// draws reports whether the caller is the goroutine the app draws on, which is
// the one the runtime started and the one a promise resolves into.
func draws() bool {
	buffer := make([]byte, 64)
	size := runtime.Stack(buffer, false)
	buffer = buffer[:size]
	const prefix = "goroutine "
	if len(buffer) <= len(prefix) {
		return false
	}
	end := len(prefix)
	for end < len(buffer) && buffer[end] != ' ' {
		end++
	}
	id, err := strconv.ParseUint(string(buffer[len(prefix):end]), 10, 64)
	return err == nil && id == 1
}

func readHeader(value js.Value) map[string]string {
	header := map[string]string{}
	collect := js.FuncOf(func(_ js.Value, args []js.Value) any {
		header[args[1].String()] = args[0].String()
		return nil
	})
	value.Call("forEach", collect)
	collect.Release()
	return header
}
