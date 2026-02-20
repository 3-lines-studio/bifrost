package core

import (
	"html/template"
)

type ErrorData struct {
	Message string
	IsDev   bool
}

var ErrorTemplate = template.Must(template.New("error").Parse(`<!doctype html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Error</title>
    <style>
        body { font-family: system-ui, sans-serif; max-width: 800px; margin: 50px auto; padding: 0 20px; }
        h1 { color: #e74c3c; }
        pre { background: #f8f9fa; padding: 15px; border-radius: 5px; overflow-x: auto; }
    </style>
</head>
<body>
    <h1>Internal Server Error</h1>
    {{if .IsDev}}
    <pre>{{.Message}}</pre>
    {{else}}
    <p>An error occurred while processing your request.</p>
    {{end}}
</body>
</html>`))
