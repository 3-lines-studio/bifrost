package renderproc

import _ "embed"

// RuntimeSource is compiled into the standalone production renderer.
//
//go:embed runtime.ts
var RuntimeSource string

// DevRuntimeSource runs Vite and the development SSR bridge under Bun.
//
//go:embed dev_runtime.ts
var DevRuntimeSource string
