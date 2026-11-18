//go:build js && wasm

package client

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"syscall/js"

	"github.com/3-lines-studio/bifrost/mygo/internal/uithread"
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
// TLS, and the build carries no HTTP stack. A request that sends a Blob, or
// reports its progress, goes with XMLHttpRequest, whose upload reports how
// much went out; fetch lends a File to it as XMLHttpRequest does.
func send(ctx context.Context, request Request) (Response, error) {
	if uithread.On() {
		return Response{}, ErrUIThread
	}
	if request.Blob.present() || request.OnUpload != nil {
		return sendProgress(ctx, request)
	}
	return sendFetch(ctx, request)
}

func sendFetch(ctx context.Context, request Request) (Response, error) {
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

// sendProgress sends the request with XMLHttpRequest, whose upload reports the
// bytes sent as they go out and takes a File as the body without reading it.
func sendProgress(ctx context.Context, request Request) (Response, error) {
	body, err := request.xhrBody()
	if err != nil {
		return Response{}, err
	}
	xhr := js.Global().Get("XMLHttpRequest").New()
	xhr.Call("open", request.Method, request.URL, true)
	xhr.Set("responseType", "arraybuffer")
	for name, value := range request.Header {
		xhr.Call("setRequestHeader", name, value)
	}

	done := make(chan outcome, 1)
	var onLoad, onFailure, onProgress js.Func
	var once sync.Once
	release := func() {
		once.Do(func() {
			onLoad.Release()
			onFailure.Release()
			if request.OnUpload != nil {
				onProgress.Release()
			}
		})
	}
	finish := func(value outcome) {
		release()
		done <- value
	}
	onLoad = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		response := Response{Status: xhr.Get("status").Int(), Header: readHeader(xhr.Call("getAllResponseHeaders"))}
		if value := xhr.Get("response"); value.Type() == js.TypeObject {
			array := js.Global().Get("Uint8Array").New(value)
			response.Body = make([]byte, array.Length())
			js.CopyBytesToGo(response.Body, array)
		}
		finish(outcome{response: response})
		return nil
	})
	onFailure = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		err := ctx.Err()
		if err == nil {
			err = errors.New("client: the request failed")
		}
		finish(outcome{err: err})
		return nil
	})
	xhr.Set("onload", onLoad)
	xhr.Set("onerror", onFailure)
	xhr.Set("onabort", onFailure)
	xhr.Set("ontimeout", onFailure)
	if request.OnUpload != nil {
		report := request.OnUpload
		onProgress = js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) == 0 || args[0].Type() != js.TypeObject {
				return nil
			}
			if loaded := args[0].Get("loaded"); loaded.Type() == js.TypeNumber {
				report(int64(loaded.Float()))
			}
			return nil
		})
		xhr.Get("upload").Call("addEventListener", "progress", onProgress)
	}

	finished := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			xhr.Call("abort")
		case <-finished:
		}
	}()
	if body.Type() == js.TypeUndefined {
		xhr.Call("send")
	} else {
		xhr.Call("send", body)
	}
	result := <-done
	close(finished)
	return result.response, result.err
}

// xhrBody is what the browser sends as the body of the request.
func (r Request) xhrBody() (js.Value, error) {
	if r.Blob.present() {
		return r.Blob.body()
	}
	if len(r.Body) == 0 {
		return js.Undefined(), nil
	}
	array := js.Global().Get("Uint8Array").New(len(r.Body))
	js.CopyBytesToJS(array, r.Body)
	return array, nil
}

// readHeader reads the headers of an answer, which fetch gives as a Headers
// and XMLHttpRequest as the text of the response.
func readHeader(value js.Value) map[string]string {
	header := map[string]string{}
	if value.Type() == js.TypeString {
		for _, line := range strings.Split(value.String(), "\r\n") {
			name, rest, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			header[strings.TrimSpace(name)] = strings.TrimSpace(rest)
		}
		return header
	}
	collect := js.FuncOf(func(_ js.Value, args []js.Value) any {
		header[args[1].String()] = args[0].String()
		return nil
	})
	value.Call("forEach", collect)
	collect.Release()
	return header
}

// Copy sends request with fetch and writes the body of the answer to w. The
// browser hands the answer over at once, so it reports the whole of it once,
// after it arrives.
func Copy(ctx context.Context, request Request, w io.Writer, onProgress func(received int64)) (int64, error) {
	response, err := Do(ctx, request)
	if err != nil {
		return 0, err
	}
	if response.Status < 200 || response.Status > 299 {
		return 0, statusError(request.Method, request.URL, response.Status)
	}
	written, err := w.Write(response.Body)
	if err != nil {
		return int64(written), err
	}
	if onProgress != nil {
		onProgress(int64(written))
	}
	return int64(written), nil
}
