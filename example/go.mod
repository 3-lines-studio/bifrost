module github.com/3-lines-studio/bifrost/example

go 1.25.6

require (
	github.com/go-chi/chi/v5 v5.2.4
	github.com/3-lines-studio/bifrost v0.0.0
)

require (
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	golang.org/x/sys v0.4.0 // indirect
)

replace github.com/3-lines-studio/bifrost => ../
