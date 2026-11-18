// Package abort drops an upload nobody finishes.
package abort

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/mygo/demo/uploadproto"
)

// Post drops the upload of the body.
func Post(w http.ResponseWriter, r *http.Request) { uploadproto.Abort(w, r) }
