.PHONY: check e2e-build test build-bifrost

build-bifrost:
	@go build -o /tmp/bifrost-build ./cmd/build/main.go

e2e-build: build-bifrost
	@cd example && /tmp/bifrost-build ./cmd/full/main.go || true

check: e2e-build
	golangci-lint run
	go test ./... -race
	go build
