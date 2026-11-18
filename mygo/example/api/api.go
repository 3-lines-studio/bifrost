// Package api is the contract between the server and the apps: the client the
// app calls for what the pages show.
package api

import (
	"context"
	"strings"

	"github.com/3-lines-studio/bifrost/mygo/client"
	"github.com/3-lines-studio/bifrost/mygo/example/store"
)

// Client calls the server from any platform: fetch in a browser, net/http
// elsewhere.
type Client struct {
	BaseURL string
}

// Notes returns every note.
func (c Client) Notes(ctx context.Context) ([]store.Note, error) {
	return client.GetJSON[[]store.Note](ctx, strings.TrimSuffix(c.BaseURL, "/")+"/api/notes", nil)
}
