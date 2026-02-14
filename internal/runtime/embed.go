package runtime

import _ "embed"

//go:embed bun_renderer_dev.ts
var BunRendererDevSource string

//go:embed bun_renderer_prod.ts
var BunRendererProdSource string
