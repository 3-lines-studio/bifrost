.PHONY: check test race vet bench fuzz integration dev-integration reproducible

check: test race vet

test:
	@test -z "$$(gofmt -l -- $$(find . -name '*.go' -not -path './example/basic/.bifrost/*'))" || (gofmt -l -- $$(find . -name '*.go' -not -path './example/basic/.bifrost/*'); exit 1)
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

bench:
	go test -run '^$$' -bench . -benchmem ./...

fuzz:
	go test -run '^$$' -fuzz FuzzParseManifest -fuzztime=10s .
	go test -run '^$$' -fuzz FuzzDocumentPath -fuzztime=10s .
	go test -run '^$$' -fuzz FuzzRawProps -fuzztime=10s .

integration:
	bun install --frozen-lockfile
	go run ./cmd/bifrost build ./example/basic
	go run ./cmd/bifrost build ./example/plugin
	bash ./scripts/vite-failure.sh
	bash ./scripts/cancellation-integration.sh
	bash ./scripts/integration.sh
	bash ./scripts/structured-integration.sh

dev-integration:
	bash ./scripts/dev-integration.sh

reproducible:
	go run ./cmd/bifrost build ./example/basic
	cp example/basic/.bifrost/manifest.json /tmp/bifrost-manifest-1.json
	go run ./cmd/bifrost build ./example/basic
	diff -u /tmp/bifrost-manifest-1.json example/basic/.bifrost/manifest.json
	rm -f /tmp/bifrost-manifest-1.json
