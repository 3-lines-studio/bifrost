module github.com/3-lines-studio/bifrost/mygo/demo

go 1.27.3

require (
	github.com/3-lines-studio/bifrost v0.0.0
	github.com/3-lines-studio/bifrost/mygo v0.0.0
	github.com/egoist/mygo v0.10.0
)

require (
	github.com/JohannesKaufmann/dom v0.3.1 // indirect
	github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.2 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/3-lines-studio/bifrost => ../../

replace github.com/3-lines-studio/bifrost/mygo => ../
