.PHONY: test test-verbose build clean help

help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

test: ## Run all tests
	go test ./...

test-verbose: ## Run all tests with verbose output
	go test -v ./...

build: ## Build the bifrost package
	go build

clean: ## Clean build artifacts
	go clean
	find . -type f -name "*.test" -delete
	find . -type d -name ".bifrost" -exec rm -rf {} + 2>/dev/null || true

check: ## Run all checks (tests + build)
	go test ./...
	go build
