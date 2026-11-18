// Package complete joins the parts of an upload the client sent.
package complete

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/mygo/demo/uploadproto"
)

// Post joins the parts of the body.
func Post(w http.ResponseWriter, r *http.Request) { uploadproto.Complete(w, r) }
