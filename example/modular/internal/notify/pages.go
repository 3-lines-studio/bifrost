package notify

import (
	"github.com/3-lines-studio/bifrost"
)

// Pages returns no pages. It is present to keep the module surface uniform.
func (m *Module) Pages() []bifrost.Route {
	return nil
}
