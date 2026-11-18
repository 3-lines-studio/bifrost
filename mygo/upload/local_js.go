//go:build js && wasm

package upload

import (
	"encoding/json"
	"syscall/js"
)

// Browser is a Store in the localStorage of the page, so a resume survives a
// reload: the FileStore of the parts a server already has.
//
// A page that keeps no storage, or one that is out of it, gets an error from
// Save, and an upload that resumes from nothing.
func Browser() Store { return browser{} }

type browser struct{}

const browserPrefix = "bifrost:upload:"

func (browser) Load(key string) (State, bool) {
	value := js.Global().Get("localStorage").Call("getItem", browserPrefix+key)
	if value.Type() != js.TypeString {
		return State{}, false
	}
	var state State
	if err := json.Unmarshal([]byte(value.String()), &state); err != nil {
		return State{}, false
	}
	return state, true
}

func (browser) Save(key string, state State) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	js.Global().Get("localStorage").Call("setItem", browserPrefix+key, string(data))
	return nil
}

func (browser) Delete(key string) error {
	js.Global().Get("localStorage").Call("removeItem", browserPrefix+key)
	return nil
}
