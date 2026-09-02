# Minimal convention app

This app only needs `page.tsx`. It has no user Go files and does not need its own `go.mod`.

```sh
go run ./cmd/bifrost dev ./example/convention-minimal
go run ./cmd/bifrost build ./example/convention-minimal
./example/convention-minimal/.bifrost/bifrost-app
```
