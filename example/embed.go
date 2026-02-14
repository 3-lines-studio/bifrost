package example

import "embed"

//go:embed all:.bifrost
var BifrostFS embed.FS

//go:embed public/icon.png
var IconPNG []byte
