// Package auth keeps the bearer token the app sends to the server.
//
// The server has no cookie jar to keep a session in, and a native window has
// no web view, so the app holds the token and every request carries it in the
// Authorization header. System stores it where the platform keeps secrets:
// the login keychain on a desktop, localStorage in a browser.
package auth

import "errors"

// ErrUnauthorized is what an app returns when the server rejects the
// credentials.
var ErrUnauthorized = errors.New("auth: wrong credentials")

// Store keeps a token between launches. Load returns "" when there is none.
type Store interface {
	Load() (string, error)
	Save(token string) error
	Clear() error
}

// Header returns the request headers that send token, nil for an empty one.
func Header(token string) map[string]string {
	if token == "" {
		return nil
	}
	return map[string]string{"Authorization": "Bearer " + token}
}
