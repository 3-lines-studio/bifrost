// Package start begins an upload: the size and name of the file, and the parts
// the server expects.
package start

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/mygo/demo/uploadproto"
)

// Post begins the upload of the file the body describes.
func Post(w http.ResponseWriter, r *http.Request) { uploadproto.Start(w, r) }
