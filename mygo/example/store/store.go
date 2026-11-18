// Package store holds what the demo serves: the notes a request reads and
// the app draws.
package store

// Note is one note.
type Note struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// All returns every note.
func All() []Note {
	return []Note{
		{ID: 1, Title: "Rendered by Go"},
		{ID: 2, Title: "Served over HTTP"},
		{ID: 3, Title: "Drawn by MyGo"},
	}
}
