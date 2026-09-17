package loadererror

import (
	"errors"
	"net/http"

	"github.com/3-lines-studio/bifrost"
)

func Load(r *http.Request) (any, error) {
	return nil, bifrost.Status(http.StatusServiceUnavailable, errors.New("the loader could not reach the upstream"))
}
