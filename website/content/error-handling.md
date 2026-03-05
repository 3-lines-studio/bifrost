---
title: Error Handling
description: Handle errors and redirects in loaders.
order: 4
---

## Loader Errors

When a loader returns an error, Bifrost responds with a 500 status and an error page:

```go
bifrost.Page("/data", "./pages/data.tsx",
    bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
        data, err := fetchData()
        if err != nil {
            return nil, err
        }
        return map[string]any{"data": data}, nil
    }),
)
```

In development, the error page shows the full error message and stack trace. In production, a generic error page is shown.

## Redirects

Return a redirect error from a loader to send the user to a different URL:

```go
bifrost.Page("/dashboard", "./pages/dashboard.tsx",
    bifrost.WithLoader(func(req *http.Request) (map[string]any, error) {
        if !isAuthenticated(req) {
            return nil, &AuthRedirect{url: "/login", status: http.StatusFound}
        }
        return map[string]any{"user": getUser(req)}, nil
    }),
)
```

Implement the `RedirectError` interface:

```go
type AuthRedirect struct {
    url    string
    status int
}

func (e *AuthRedirect) Error() string            { return "redirect" }
func (e *AuthRedirect) RedirectURL() string       { return e.url }
func (e *AuthRedirect) RedirectStatusCode() int   { return e.status }
```

Any error implementing `RedirectURL()` and `RedirectStatusCode()` triggers a redirect instead of an error page.

## Production Startup Errors

Bifrost panics at startup if production requirements are not met:

- Missing `embed.FS`
- Missing `manifest.json` in embedded assets
- Missing Bun runtime (for apps with SSR pages)

This fail-fast behavior ensures problems are caught immediately at deployment rather than at runtime.
