module github.com/3-lines-studio/bifrost/website

go 1.25.6

replace github.com/3-lines-studio/bifrost => ../

require (
	github.com/3-lines-studio/bifrost v0.0.0-00010101000000-000000000000
	github.com/yuin/goldmark v1.7.16
	github.com/yuin/goldmark-meta v1.1.0
)

require gopkg.in/yaml.v2 v2.3.0 // indirect
