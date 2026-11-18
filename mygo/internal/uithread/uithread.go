// Package uithread tells whether code runs on the goroutine the app draws on.
//
// In a browser, promises resolve on the goroutine the runtime started, which
// is the one that draws: code that waits there for an answer that arrives as a
// promise waits forever. Code that offers a blocking call asks On first.
package uithread

// On reports whether the caller runs on the goroutine that draws. It is false
// off the browser, where nothing waits for a promise.
func On() bool { return on() }
