//go:build !js

package auth

import (
	"errors"
	"strings"

	"github.com/egoist/mygo"
)

// System stores the token in the platform's secret store: the login keychain
// on macOS, libsecret on Linux, the Credential Manager on Windows. A backend
// without one answers errors.ErrUnsupported.
func System() Store {
	return keychain{}
}

type keychain struct{}

var secrets = mygo.Secrets.Service("bifrost")

func (keychain) Load() (string, error) {
	value, err := secrets.Get("token")
	if errors.Is(err, mygo.ErrSecretNotFound) || errors.Is(err, errors.ErrUnsupported) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(value)), nil
}

func (keychain) Save(token string) error {
	return secrets.Set("token", []byte(token))
}

func (keychain) Clear() error {
	err := secrets.Delete("token")
	if errors.Is(err, errors.ErrUnsupported) {
		return nil
	}
	return err
}
