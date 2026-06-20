package core

import (
	"fmt"
	"html/template"
	"strings"
)

type StructuredError struct {
	ErrorType string
	Message   string
	Stack     string
	File      string
	Line      int
	Column    int
	LineText  string
	Specifier string
	Referrer  string
	SubErrors []StructuredError
}

func (e *StructuredError) Error() string {
	var sb strings.Builder
	sb.WriteString(e.Message)
	if len(e.SubErrors) > 0 {
		sb.WriteString("\n\nErrors:")
		for i, sub := range e.SubErrors {
			fmt.Fprintf(&sb, "\n  %d. %s", i+1, sub.Message)
			if sub.File != "" {
				fmt.Fprintf(&sb, " (%s:%d:%d)", sub.File, sub.Line, sub.Column)
			}
		}
	}
	if e.Stack != "" {
		fmt.Fprintf(&sb, "\n\nStack:\n%s", e.Stack)
	}
	return sb.String()
}

type ErrorData struct {
	Message     string
	IsDev       bool
	Structured  *StructuredError
	CodeSnippet string
	NextSteps   []string
}

var ErrorTemplate = template.Must(template.New("error").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{if .Structured}}{{.Structured.ErrorType}} - Bifrost Dev{{else}}Error{{end}}</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: ui-monospace, SFMono-Regular, monospace;
  background: #0a0a0a;
  color: #f5f5f5;
  min-height: 100vh;
  display: flex;
  justify-content: center;
  padding: 40px 20px;
}
.container { max-width: 900px; width: 100%; }
.badge {
  display: inline-block;
  padding: 4px 12px;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  margin-bottom: 16px;
}
.badge-build { background: #f59e0b22; color: #f59e0b; border: 1px solid #f59e0b44; }
.badge-render { background: #ef444422; color: #ef4444; border: 1px solid #ef444444; }
.badge-import { background: #8b5cf622; color: #8b5cf6; border: 1px solid #8b5cf644; }
h1 { font-size: 1.25rem; font-weight: 700; color: #ff5555; margin-bottom: 8px; word-break: break-word; }
h2 { font-size: 0.9375rem; font-weight: 600; color: #888; margin: 24px 0 8px 0; text-transform: uppercase; letter-spacing: 0.05em; }
.location { color: #666; font-size: 0.875rem; margin-bottom: 16px; font-family: ui-monospace, SFMono-Regular, monospace; }
.location strong { color: #aaa; }
pre {
  background: #111;
  border: 1px solid #333;
  padding: 16px;
  border-radius: 4px;
  overflow-x: auto;
  font-size: 0.8125rem;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}
.code-snippet {
  border-left: 3px solid #ff5555;
  padding-left: 12px;
  margin: 8px 0 16px 0;
}
.line-num { color: #555; user-select: none; margin-right: 12px; }
.line-highlight { color: #ff8888; }
details { margin: 8px 0; }
summary { color: #888; cursor: pointer; font-size: 0.875rem; padding: 4px 0; }
summary:hover { color: #aaa; }
.sub-error { margin: 8px 0; padding: 12px; background: #0d0d0d; border: 1px solid #222; border-radius: 4px; }
.sub-error .sub-msg { color: #f5f5f5; font-size: 0.875rem; }
.sub-error .sub-loc { color: #666; font-size: 0.75rem; margin-top: 4px; }
.next-steps { list-style: none; padding: 0; }
.next-steps li { padding: 6px 0; color: #aaa; font-size: 0.875rem; border-bottom: 1px solid #1a1a1a; }
.next-steps li:last-child { border-bottom: none; }
.next-steps li::before { content: "→ "; color: #ff5555; }
.info-tag { color: #666; font-size: 0.75rem; margin-right: 8px; }
.info-val { color: #aaa; font-size: 0.875rem; font-family: ui-monospace, SFMono-Regular, monospace; }
p { color: #999; font-size: 0.9375rem; line-height: 1.6; }
</style>
</head>
<body>
<div class="container">
{{if .Structured}}
  {{if eq .Structured.ErrorType "Build Error"}}
    <span class="badge badge-build">Build Error</span>
  {{else if eq .Structured.ErrorType "Render Error"}}
    <span class="badge badge-render">Render Error</span>
  {{else if eq .Structured.ErrorType "Import Error"}}
    <span class="badge badge-import">Import Error</span>
  {{else}}
    <span class="badge badge-build">{{.Structured.ErrorType}}</span>
  {{end}}
  <h1>{{.Structured.Message}}</h1>
  {{if .Structured.File}}
    <div class="location"><strong>{{.Structured.File}}</strong>:{{.Structured.Line}}:{{.Structured.Column}}</div>
  {{end}}
  {{if .CodeSnippet}}
    <h2>Code</h2>
    <pre class="code-snippet"><span class="line-num">{{.Structured.Line}}</span><span class="line-highlight">{{.CodeSnippet}}</span></pre>
  {{end}}
  {{if .Structured.Specifier}}
    <h2>Import</h2>
    <div><span class="info-tag">Specifier:</span><span class="info-val">{{.Structured.Specifier}}</span></div>
    {{if .Structured.Referrer}}<div><span class="info-tag">Referrer:</span><span class="info-val">{{.Structured.Referrer}}</span></div>{{end}}
  {{else if .Structured.Referrer}}
    <div><span class="info-tag">Referrer:</span><span class="info-val">{{.Structured.Referrer}}</span></div>
  {{end}}
  {{if .Structured.Stack}}
    <h2>Stack Trace</h2>
    <details open>
      <summary>Show stack ({{len .Structured.Stack}} bytes)</summary>
      <pre>{{.Structured.Stack}}</pre>
    </details>
  {{end}}
  {{if .Structured.SubErrors}}
    <h2>Sub-Errors</h2>
    {{range $i, $sub := .Structured.SubErrors}}
    <div class="sub-error">
      <div class="sub-msg">{{$sub.Message}}</div>
      {{if $sub.File}}<div class="sub-loc">{{$sub.File}}:{{$sub.Line}}:{{$sub.Column}}</div>{{end}}
    </div>
    {{end}}
  {{end}}
  {{if .NextSteps}}
    <h2>Next Steps</h2>
    <ul class="next-steps">
    {{range .NextSteps}}
      <li>{{.}}</li>
    {{end}}
    </ul>
  {{end}}
{{else if .IsDev}}
  <h1>Internal Server Error</h1>
  <pre>{{.Message}}</pre>
{{else}}
  <h1>Internal Server Error</h1>
  <p>An error occurred while processing your request.</p>
{{end}}
</div>
</body>
</html>`))
