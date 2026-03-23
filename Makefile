
check:
	go run ./cmd/doctor ./example
	go build -o /tmp/bifrost-build ./cmd/build/main.go
	cd example && bun i && /tmp/bifrost-build ./cmd/full/main.go
	env GOCACHE=/tmp/bifrost-go-build-cache GOMODCACHE=/tmp/bifrost-go-mod-cache GOPATH=/tmp/bifrost-go-path GOLANGCI_LINT_CACHE=/tmp/bifrost-golangci-lint-cache golangci-lint run ./...
	go test ./... -race
	cd test/e2e && go test ./...
	go build
