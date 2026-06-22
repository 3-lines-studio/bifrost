package main

import "embed"

//go:embed all:.bifrost all:public
var BifrostFS embed.FS
