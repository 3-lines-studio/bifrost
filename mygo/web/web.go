// Package web serves a MyGo WebAssembly build with the HTML of its pages
// already in it, so crawlers read the content and people get the app.
//
// Shell returns the body of a page: the content for crawlers, styled until the
// app draws, and the script that boots the app from /app.wasm over it, at the
// page the URL names.
package web

import "strings"

const (
	execPath = "/wasm_exec.js"
	wasmPath = "/app.wasm"
)

// ReloadPath is where ReloadHandler answers the stream the Reload script of a
// page follows.
const ReloadPath = "/__bifrost/reload"

// style dresses the content before the app runs, so the HTML a crawler reads
// and a person lands on looks like a page and not a column of text. The app
// replaces it with its own drawing.
const style = `html,body{margin:0;height:100%;color-scheme:light dark}` +
	`#seo{max-width:42rem;margin:0 auto;padding:24px 20px;font:16px/1.6 system-ui,-apple-system,"Segoe UI",sans-serif}` +
	`#seo h1,#seo h2,#seo h3{line-height:1.2;margin:1.4em 0 .5em}#seo h1{margin-top:0;font-size:2em}` +
	`#seo h2{font-size:1.4em}#seo p{margin:0 0 1em}#seo a{color:LinkText}#seo main{display:block}` +
	`#mygo{position:fixed;inset:0;display:none;background:Canvas}` +
	`#mygo-loading{position:fixed;inset:0;display:none;flex-direction:column;align-items:center;justify-content:center;gap:12px;background:Canvas;color:CanvasText;z-index:1}` +
	`.mygo-spinner{width:24px;height:24px;border:2px solid rgba(128,128,128,.35);border-top-color:currentColor;border-radius:50%;animation:mygo-spin .8s linear infinite}` +
	`@keyframes mygo-spin{to{transform:rotate(360deg)}}` +
	`@media (prefers-reduced-motion:reduce){.mygo-spinner{animation:none}}`

// The boot script starts the app at the URL's path: MyGo's first router
// follows the tab's history in the fragment, so /pricing becomes #/pricing.
// The content stays until the app runs, and stays for good when the build is
// missing, so a failed fetch degrades to the HTML. The spinner waits 400ms
// before showing, so a cached build does not flash it.
const script = `(function(){` +
	`var seo=document.getElementById("seo");` +
	`var loader=document.getElementById("mygo-loading");` +
	`var root=document.getElementById("mygo");` +
	`if(location.pathname!=="/"&&!location.hash){history.replaceState(null,"","#"+location.pathname)}` +
	`var timer=setTimeout(function(){if(loader){loader.style.display="flex"}},400);` +
	`if(typeof Go==="undefined"){clearTimeout(timer);return}` +
	`var go=new Go();` +
	`WebAssembly.instantiateStreaming(fetch("` + wasmPath + `"),go.importObject).then(function(result){` +
	`clearTimeout(timer);` +
	`go.run(result.instance);` +
	`root.style.display="block";` +
	`if(loader){loader.remove()}` +
	`if(seo){seo.remove()}` +
	`}).catch(function(){` +
	`clearTimeout(timer);` +
	`if(loader){loader.remove()}` +
	`})})();`

// The reload script follows the server: the stream sends the id of the process
// when it connects, and a page that reads another one reloads. A dev server
// builds and restarts on every edit, so the page follows the edit; a page
// served by a build that has no stream at all never asks.
const reloadScript = `(function(){` +
	`if(typeof EventSource==="undefined"){return}` +
	`var seen=null;` +
	`var stream=new EventSource("` + ReloadPath + `");` +
	`stream.onmessage=function(event){` +
	`if(seen===null){seen=event.data;return}` +
	`if(seen!==event.data){stream.close();location.reload()}` +
	`}})();`

// Option changes what the shell carries.
type Option func(*shell)

type shell struct {
	reload bool
}

// Reload makes the page follow the server: when the process answering it is
// another one, the page reloads. It is what shows an edit in the browser while
// `bifrost dev` runs, and a build leaves it out.
func Reload() Option {
	return func(s *shell) {
		s.reload = true
	}
}

// Shell returns the body of a page showing content, or nil content for a page
// the app draws alone. Serve /wasm_exec.js and /app.wasm beside it, with
// FileServer.
func Shell(content []byte, options ...Option) []byte {
	var settings shell
	for _, option := range options {
		option(&settings)
	}
	var body strings.Builder
	if len(content) > 0 {
		body.WriteString(`<div id="seo">`)
		body.Write(content)
		body.WriteString(`</div>`)
	}
	body.WriteString(`<div id="mygo"></div><div id="mygo-loading" role="status"><div class="mygo-spinner"></div></div><style>`)
	body.WriteString(style)
	body.WriteString(`</style><script src="`)
	body.WriteString(execPath)
	body.WriteString(`"></script><script>`)
	body.WriteString(script)
	body.WriteString(`</script>`)
	if settings.reload {
		body.WriteString(`<script>`)
		body.WriteString(reloadScript)
		body.WriteString(`</script>`)
	}
	return []byte(body.String())
}
