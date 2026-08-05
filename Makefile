check:
	go run ./cmd/bifrost doctor ./example/cmd/full
	go build -o /tmp/bifrost ./cmd/bifrost
	cd example && bun i && /tmp/bifrost build ./cmd/full/main.go
	mkdir -p test/e2e/.bifrost test/e2e/public
	cp -r example/cmd/full/.bifrost/. test/e2e/.bifrost/
	cp -r example/public/. test/e2e/public/
	env GOCACHE=/tmp/bifrost-go-build-cache GOMODCACHE=/tmp/bifrost-go-mod-cache GOPATH=/tmp/bifrost-go-path GOLANGCI_LINT_CACHE=/tmp/bifrost-golangci-lint-cache golangci-lint run ./...
	go test ./... -race
	cd test/e2e && go test ./...
	go build
