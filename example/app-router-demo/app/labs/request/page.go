package request

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	store "github.com/3-lines-studio/bifrost/example/app-router-demo/app/store"
)

func Load(r *http.Request) (any, error) {
	id := make([]byte, 6)
	_, _ = rand.Read(id)
	return map[string]any{
		"id":    hex.EncodeToString(id),
		"loads": store.Loads("/labs/request"),
		"path":  r.URL.Path,
	}, nil
}
