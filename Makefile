.PHONY: check bench bench-sobek profile-sobek release-check

check:
	go run ./cmd/bifrost doctor ./example/cmd/full
	go build -o /tmp/bifrost ./cmd/bifrost
	cd example && bun i && BIFROST_JS_RUNTIME=bun /tmp/bifrost build ./cmd/full/main.go
	cd example && PATH="$$(go env GOROOT)/bin:/usr/bin:/bin" /tmp/bifrost build ./cmd/full/main.go
	mkdir -p test/e2e/.bifrost
	cp -r example/cmd/full/.bifrost/. test/e2e/.bifrost/
	env GOCACHE=/tmp/bifrost-go-build-cache GOMODCACHE=/tmp/bifrost-go-mod-cache GOPATH=/tmp/bifrost-go-path GOLANGCI_LINT_CACHE=/tmp/bifrost-golangci-lint-cache golangci-lint run ./...
	go test ./... -race
	cd example && go test ./...
	cd test/e2e && PATH="$$(go env GOROOT)/bin:/usr/bin:/bin" go test ./...
	go build

bench:
	go test ./internal/adapters/process -run '^$$' -bench '^Benchmark(Renderer|ReactEngine|Runtime)' -benchmem -count=3

bench-sobek:
	$(MAKE) -C example build-sobek
	go test ./internal/adapters/process -run '^$$' -bench '^BenchmarkRuntimeRealPage' -benchmem -benchtime=2s -count=3
	go test ./internal/adapters/sobek -run '^$$' -bench '^BenchmarkSobek' -benchmem -benchtime=2s -count=3

profile-sobek:
	$(MAKE) -C example build-sobek
	go test ./internal/adapters/process -run '^$$' -bench '^BenchmarkRuntimeRealPageSerial/Sobek$$' -benchtime=5s -count=1 -cpuprofile=/tmp/bifrost-sobek-cpu.pprof -memprofile=/tmp/bifrost-sobek-mem.pprof
	rm -f process.test
	go tool pprof -top -nodecount=30 /tmp/bifrost-sobek-cpu.pprof
	go tool pprof -top -alloc_space -nodecount=30 /tmp/bifrost-sobek-mem.pprof

release-check: check
	go vet ./...
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
	bun audit
	git diff --check
