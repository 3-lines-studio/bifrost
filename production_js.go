//go:build js

package bifrost

import "errors"

func (a *App) initializeProduction(config Config) error {
	return errors.New("bifrost: embedded assets and the Bun renderer are not supported on js/wasm")
}
