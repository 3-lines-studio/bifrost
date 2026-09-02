# Full convention app

This example exercises pages, loaders, dynamic params, every REST method, nested middleware, layouts, error fallbacks, not-found pages, public files, custom Vite config, streaming failure semantics, and `server.go`.

```sh
go run ./cmd/bifrost dev ./example/convention
```

Use `BIFROST_ADDR` because this example owns the server through `server.go`:

```sh
BIFROST_ADDR=127.0.0.1:9000 go run ./cmd/bifrost dev ./example/convention
```

Build and run:

```sh
go run ./cmd/bifrost build ./example/convention
BIFROST_ADDR=127.0.0.1:9000 ./example/convention/.bifrost/bifrost-app
```

Try every REST method:

```sh
for method in GET POST PUT PATCH DELETE HEAD OPTIONS; do
  curl -i -X "$method" http://localhost:8080/posts/api/hello
done
```
