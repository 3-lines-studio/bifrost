package renderproc

import _ "embed"

//go:embed runtime.ts
var RuntimeSource string

//go:embed dev_runtime.ts
var DevRuntimeSource string
