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
        .container {
            width: 100%;
        }
        h1 {
        		font-size: 1.2rem;
            font-weight: bold;
            color: #ff5555;
            margin-bottom: 24px;
        }
        pre {
            background: #111111;
            border: 1px solid #222222;
            padding: 20px;
            border-radius: 4px;
            overflow-x: auto;
            font-size: 0.875rem;
            line-height: 1.6;
        }
        p {
            color: #999999;
            font-size: 0.9375rem;
            line-height: 1.6;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Internal Server Error</h1>
        {{if .IsDev}}
        <pre>{{.Message}}</pre>
        {{else}}
        <p>An error occurred while processing your request.</p>
        {{end}}
    </div>
</body>
</html>`))
