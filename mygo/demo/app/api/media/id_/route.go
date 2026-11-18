// Package media is the file a finished upload left, at the path its share link
// names.
package media

import (
	"net/http"

	"github.com/3-lines-studio/bifrost/mygo/demo/uploadproto"
)

// Get answers the bytes of the file.
func Get(w http.ResponseWriter, r *http.Request) { uploadproto.Media(w, r) }
