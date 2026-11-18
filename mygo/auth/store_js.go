//go:build js && wasm

package auth

import (
	"errors"
	"syscall/js"
)

const key = "bifrost.token"

// System stores the token in the browser's localStorage.
func System() Store {
	return local{}
}

type local struct{}

func (local) Load() (string, error) {
	storage := js.Global().Get("localStorage")
	if storage.IsUndefined() {
		return "", nil
	}
	value := storage.Call("getItem", key)
	if value.IsNull() || value.IsUndefined() {
		return "", nil
	}
	return value.String(), nil
}

func (local) Save(token string) error {
	storage := js.Global().Get("localStorage")
	if storage.IsUndefined() {
		return errors.New("auth: localStorage is not available")
	}
	storage.Call("setItem", key, token)
	return nil
}

func (local) Clear() error {
	storage := js.Global().Get("localStorage")
	if storage.IsUndefined() {
		return nil
	}
	storage.Call("removeItem", key)
	return nil
}
