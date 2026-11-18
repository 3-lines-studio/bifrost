// Package sign answers the URL of each part of an upload.
package sign

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/mygo/demo/uploadproto"
)

// Post signs the parts the body asks for.
func Post(w http.ResponseWriter, r *http.Request) { uploadproto.Sign(w, r) }
