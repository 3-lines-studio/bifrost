
check:
	go run ./cmd/doctor ./example
	go run ./cmd/doctor ./example-svelte
	go build -o /tmp/bifrost-build ./cmd/build/main.go
	cd example && bun i && /tmp/bifrost-build ./cmd/full/main.go || true
	cd example-svelte && bun i && /tmp/bifrost-build --framework svelte ./cmd/full/main.go || true
	golangci-lint run
	go test ./... -race
	cd test/e2e && go test ./...
	go build
