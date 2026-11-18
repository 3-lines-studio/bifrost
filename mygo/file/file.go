// Package file hands the app the files of the person using it: the ones picked
// in a window or in the browser, and the ones the app opens by path.
//
// A File is read in ranges, so a picture of 40MiB goes to the server in parts
// without being read into Go first: Blob returns a part a request sends where
// it is, and Head reads the bytes a decoder needs to know what the file is.
//
// Pick, Head and Download wait for the person and for the browser, so they are
// called from a goroutine, as client calls are: in a browser they answer
// ErrUIThread when called where the app draws.
package file

import (
	"mime"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

// Options say what Pick offers.
type Options struct {
	// Title is the title of the window's dialog. Browsers show none.
	Title string
	// Multiple lets the person pick several files.
	Multiple bool
	// Filters limit what the picker shows, as MyGo's dialogs take them. In a
	// browser only the first filter becomes the input's accept attribute.
	Filters []mygo.FileFilter
}

// Download is a file the app saves from the server.
type Download struct {
	URL    string
	Header map[string]string
	// Name is the name the file gets. A window asks where to save it; a
	// browser puts it in its downloads.
	Name string
	// OnProgress is called with the bytes received so far, as they come. A
	// browser downloads the file itself, so it reports none.
	OnProgress func(received int64)
}

func contentType(name string, given string) string {
	if given != "" {
		return given
	}
	if kind := mime.TypeByExtension(filepath.Ext(name)); kind != "" {
		return kind
	}
	return "application/octet-stream"
}

// validRange reports whether the bytes from start to end are inside a file of
// size bytes.
func validRange(start, end, size int64) bool {
	return start >= 0 && end >= start && end <= size
}

// accept turns the filters into the accept attribute of a browser's input.
func accept(filters []mygo.FileFilter) string {
	if len(filters) == 0 {
		return ""
	}
	filter := filters[0]
	values := make([]string, 0, len(filter.MIMETypes)+len(filter.Extensions))
	values = append(values, filter.MIMETypes...)
	for _, extension := range filter.Extensions {
		if value, ok := browserExtension(extension); ok {
			values = append(values, value)
		}
	}
	return joinComma(values)
}

func joinComma(values []string) string {
	out := ""
	for _, value := range values {
		if out != "" {
			out += ","
		}
		out += value
	}
	return out
}

// browserExtension turns an extension of a filter, as ".jpg", "jpg" or
// "*.jpg", into the one an input's accept takes, and reports false for "*",
// which every file of the filter takes.
func browserExtension(extension string) (string, bool) {
	extension = strings.TrimPrefix(extension, "*")
	if extension == "" || extension == "." {
		return "", false
	}
	return "." + strings.TrimPrefix(extension, "."), true
}
