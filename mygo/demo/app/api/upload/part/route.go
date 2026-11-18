// Package part keeps the bytes of one part of an upload.
package part

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/mygo/demo/uploadproto"
)

// Put keeps the part of the body and answers its ETag.
func Put(w http.ResponseWriter, r *http.Request) { uploadproto.Part(w, r) }
