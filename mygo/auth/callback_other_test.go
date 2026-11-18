//go:build !js

package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestListen(t *testing.T) {
	callback, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	address, err := url.Parse(callback.URL())
	if err != nil {
		t.Fatal(err)
	}
	if address.Hostname() != "127.0.0.1" || address.Path != "/callback" {
		t.Fatalf("url %q", callback.URL())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	answered := make(chan url.Values, 1)
	go func() {
		answer, err := callback.Wait(ctx)
		if err != nil {
			t.Errorf("wait: %v", err)
			close(answered)
			return
		}
		answered <- answer
	}()

	response, err := http.Get(callback.URL() + "?code=abc&state=xyz")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status %d", response.StatusCode)
	}
	select {
	case answer := <-answered:
		if answer == nil {
			t.Fatal("no answer")
		}
		if answer.Get("code") != "abc" || answer.Get("state") != "xyz" {
			t.Fatalf("answer %v", answer)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the callback did not answer")
	}
}

func TestListenOnlyOnLoopback(t *testing.T) {
	if _, err := Listen("0.0.0.0:0"); err == nil {
		t.Fatal("listened on every address")
	}
}

func TestWaitEndsWithTheContext(t *testing.T) {
	callback, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := callback.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait answered %v", err)
	}
}
