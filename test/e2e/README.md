# E2E Tests for Bifrost

This directory contains comprehensive end-to-end tests for the Bifrost library using **browserless testing with snapshot verification**.

## Approach: Browserless + Snapshot Testing

Instead of using a browser automation tool (like Chrome/chromedp), these tests use Go's `http.Client` to fetch HTML responses and verify them against stored snapshots. This provides:

✅ **No browser required** - Pure Go, works anywhere  
✅ **Fast execution** - No browser startup overhead  
✅ **Portable** - No system dependencies  
✅ **Great regression detection** - Snapshots catch any HTML changes  
✅ **Reviewable** - PRs show HTML diffs clearly  

## Test Coverage

### 1. SSR Pages (`ssr_test.go`)
Tests server-side rendered pages with data loaders:
- **Home page** (`/`): Basic SSR with props
- **Nested pages** (`/nested`): Directory-based routing
- **Dynamic params** (`/message/{id}`): URL parameter extraction
- **API Demo** (`/api-demo`): SSR with server data

### 2. Client-Only Pages (`client_test.go`)
Tests client-side rendered pages (empty shell + hydration):
- **About page** (`/about`): Basic client-only page
- **Login page** (`/login`): Form handling

### 3. Static Pages (`static_test.go`)
Tests static generation:
- **Product page** (`/product`): Static prerender
- **Blog pages** (`/blog/{slug}`): Dynamic static paths

### 4. Errors & Redirects (`errors_test.go`)
Tests error handling and redirects:
- **RedirectError**: Authentication redirects (302)
- **Authenticated access**: Successful dashboard access
- **Loader errors**: 500 errors from data loaders
- **Render errors**: Runtime render failures
- **Import errors**: Module loading failures

## Running Tests

### Prerequisites
- Go 1.25+
- Bun installed and available in PATH

### Run all E2E tests (recommended)
```bash
make test
```

This will:
1. Build the bifrost-build tool
2. Build example assets (`make e2e-build`)
3. Run all E2E tests

### Run tests directly (requires built assets)
```bash
# First, build the example assets
cd example && go run ../cmd/build/main.go ./cmd/full/main.go

# Then run tests
go test ./test/e2e/... -v
```

### Run specific test file
```bash
go test ./test/e2e/... -v -run TestSSR
go test ./test/e2e/... -v -run TestClient
go test ./test/e2e/... -v -run TestStatic
go test ./test/e2e/... -v -run TestRedirect
```

### Run tests for specific mode
```bash
# Development mode only
go test ./test/e2e/... -v -run "_Dev$"

# Production mode only (requires built assets)
go test ./test/e2e/... -v -run "_Prod$"
```

### Update snapshots
If you intentionally change the HTML output, update snapshots with:
```bash
UPDATE_SNAPS=true go test ./test/e2e/... -v
```

## Test Architecture

### Infrastructure (`helper_test.go`)
- **testServer**: Manages Bifrost app lifecycle and HTTP client
- **HTTP-based assertions**: Status codes, redirects, content
- **HTML normalization**: Removes dynamic content (timestamps, IDs) for stable snapshots
- **Automatic cleanup**: Servers cleaned up after each test
- **Dual mode testing**: Tests run in both Dev and Production modes using `example.BifrostFS`

### Key Features
1. **Dev mode testing**: Tests run in development mode against example pages
2. **Production mode testing**: Tests run against built/embedded assets
3. **Snapshot verification**: HTML output compared against stored snapshots
4. **HTTP assertions**: Status codes, redirects, headers
5. **Normalization**: Dynamic content normalized for stable snapshots

### HTML Normalization
The `normalizeHTML()` function removes dynamic content that changes between runs:
- React hydration IDs (`data-rid`)
- CSRF tokens and nonces
- Timestamps (ISO and standard formats)
- Build hashes in asset URLs
- Random element IDs
- Whitespace normalization

### Snapshots
Snapshots are stored in `__snapshots__/` directories alongside test files:
- `helper_test.snap.html` - All test snapshots (24 total)

Snapshots are plain text files with normalized HTML, making them easy to review in PRs.

## Adding New Tests

To add a new E2E test:

1. Create a test function in the appropriate file
2. Set up routes using `bifrost.Page()`
3. Create a test server with `newTestServer(t, routes, true)` (dev mode) or `newTestServer(t, routes, false)` (production mode)
4. Make HTTP requests with `server.get(t, "/path")`
5. Assert status codes with `assertHTTPStatus(t, resp, 200)`
6. Match HTML against snapshots with `matchSnapshot(t, "name", html)`

Example:
```go
func TestMyPage_Dev(t *testing.T) {
    skipIfNoBun(t)

    routes := []bifrost.Route{
        bifrost.Page("/my-page", "./pages/my-page.tsx", 
            bifrost.WithLoader(func(*http.Request) (map[string]any, error) {
                return map[string]any{"title": "My Page"}, nil
            }),
        ),
    }

    server := newTestServer(t, routes, true)
    server.start(t)

    resp, html := server.get(t, "/my-page")
    assertHTTPStatus(t, resp, 200)

    matchSnapshot(t, "my_page_dev", html)
}
```

## CI/CD Integration

These tests are designed for CI environments:
- Automatically skip if Bun is not available
- No browser or display required
- Fast execution (~10 seconds for all 24 tests)
- Fail on snapshot mismatches
- Use `make test` to ensure assets are built before testing

### Makefile Targets

- `make e2e-build` - Build example assets for production testing
- `make test` - Build assets and run all E2E tests
- `make check` - Run full check suite (lint, test, build)

## Notes

- Tests require the example pages in `/example/` directory
- Dev mode tests exercise the rendering engine with source files
- Production mode tests validate actual production behavior with embedded assets
- Snapshots should be committed to version control
- Review snapshot changes carefully in PRs - they indicate HTML changes
- Production tests will fail if example assets haven't been built
