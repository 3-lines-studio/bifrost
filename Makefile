.PHONY: check test race vet gate demo native-html bench fuzz throughput integration dev-integration reproducible

check: test race vet gate native-html demo

test:
	@test -z "$$(gofmt -l -- $$(find . -name '*.go' -not -path './example/basic/.bifrost/*'))" || (gofmt -l -- $$(find . -name '*.go' -not -path './example/basic/.bifrost/*'); exit 1)
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

demo:
	$(MAKE) -C example/app-router-demo check

gate:
	bun install --frozen-lockfile
	BIFROST_RELEASE_GATE=1 go test ./cmd/bifrost -run TestAppRouterReleaseGate -count=1

# The parser of injected HTML is generated as a string of Go, so nothing else
# executes it.
native-html:
	bun ./scripts/native-html.mjs

bench:
	go test -run '^$$' -bench . -benchmem ./...

fuzz:
	go test -run '^$$' -fuzz FuzzParseManifest -fuzztime=10s .
	go test -run '^$$' -fuzz FuzzDocumentPath -fuzztime=10s .
	go test -run '^$$' -fuzz FuzzRawProps -fuzztime=10s .

throughput:
	bash ./scripts/throughput.sh

integration:
	bun install --frozen-lockfile
	go run ./cmd/bifrost build ./example/basic
	go run ./cmd/bifrost build ./example/plugin
	bash ./scripts/vite-failure.sh
	bash ./scripts/cancellation-integration.sh
	bash ./scripts/integration.sh
	bash ./scripts/navigation-integration.sh
	bash ./scripts/structured-integration.sh
	bun ./scripts/native-html.mjs

dev-integration:
	bash ./scripts/dev-integration.sh

reproducible:
	go run ./cmd/bifrost build ./example/basic
	cp example/basic/.bifrost/manifest.json /tmp/bifrost-manifest-1.json
	go run ./cmd/bifrost build ./example/basic
	diff -u /tmp/bifrost-manifest-1.json example/basic/.bifrost/manifest.json
	rm -f /tmp/bifrost-manifest-1.json
