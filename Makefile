test: ## Run all tests
	go test ./...

check:
	golangci-lint run
	go test ./...
	go build
