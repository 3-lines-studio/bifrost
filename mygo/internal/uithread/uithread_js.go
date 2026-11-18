//go:build js

package uithread

import (
	"runtime"
	"strconv"
)

// on parses the id of the calling goroutine out of the stack header: the
// runtime starts the one that draws, which is the first.
func on() bool {
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
