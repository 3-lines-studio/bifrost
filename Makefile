
check:
	go run ./cmd/doctor ./example
	go build -o /tmp/bifrost-build ./cmd/build/main.go
	cd example && /tmp/bifrost-build ./cmd/full/main.go || true
	golangci-lint run
	go test ./... -race
	go build
