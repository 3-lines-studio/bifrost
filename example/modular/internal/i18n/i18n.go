package i18n

import (
	"github.com/3-lines-studio/bifrost/example/modular/internal/config"
)

// Module is a leaf module. It owns the message catalog and translation. Other
// modules depend on it and read value for a locale.
type Module struct {
	defaultLocale string
}

func New() *Module { return &Module{} }

func (m *Module) Wire(cfg *config.Module) {
	m.defaultLocale = cfg.Value().I18n.Default
}

func (m *Module) Default() string { return m.defaultLocale }

// Value returns the translated string for key in locale, falling back to the
// default locale when the key or locale is missing.
func (m *Module) Value(locale, key string) string {
	// Stub. Replace with a real catalog lookup in production.
	return key
}
