//go:build !js

package upload

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Disk is a Store in a directory of the app, one JSON file per upload, so a
// resume survives the app closing: the FileStore of the parts a server already
// has.
//
// It is safe for concurrent use.
func Disk(dir string) Store {
	return &disk{dir: dir}
}

type disk struct {
	dir string
	mu  sync.Mutex
}

func (d *disk) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(d.dir, hex.EncodeToString(sum[:16])+".json")
}

// Load returns the state of key, and false when it keeps none.
func (d *disk) Load(key string) (State, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	data, err := os.ReadFile(d.path(key))
	if err != nil {
		return State{}, false
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, false
	}
	return state, true
}

// Save keeps the state of key, written whole or not at all.
func (d *disk) Save(key string, state State) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := os.MkdirAll(d.dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(d.dir, "upload-*.json")
	if err != nil {
		return err
	}
	temp := file.Name()
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(temp)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(temp)
		return err
	}
	return os.Rename(temp, d.path(key))
}

// Delete forgets key.
func (d *disk) Delete(key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	err := os.Remove(d.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
