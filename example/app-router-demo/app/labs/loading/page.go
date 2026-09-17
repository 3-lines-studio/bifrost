package loading

import (
	"net/http"
	"time"
)

func Load(r *http.Request) (any, error) {
	time.Sleep(700 * time.Millisecond)
	return map[string]any{"slept": "700ms"}, nil
}
