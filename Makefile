.PHONY: check test race vet gate demo native-html native-row native-selectors native-events bench fuzz throughput integration dev-integration reproducible

check: test race vet gate native-html native-row native-selectors native-events demo

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

# The pass that gives a flex container the direction a browser gives it is
# generated as a string of Go, so nothing else executes it.
native-row:
	bun ./scripts/native-row.mjs

# The pass that takes the tag of a selector down to a class is a string of Go
# too, and it is the one that decides what a stylesheet written for a browser
# means on a native view.
native-selectors:
	bun ./scripts/native-selectors.mjs

# The reader of a stream is a string of Go too, and it is the one that decides
# what a page that watches its own server sees and when.
native-events:
	bun ./scripts/native-events.mjs

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
	bun ./scripts/native-row.mjs
	bun ./scripts/native-selectors.mjs
	bun ./scripts/native-events.mjs

dev-integration:
	bash ./scripts/dev-integration.sh

reproducible:
	go run ./cmd/bifrost build ./example/basic
	cp example/basic/.bifrost/manifest.json /tmp/bifrost-manifest-1.json
	go run ./cmd/bifrost build ./example/basic
	diff -u /tmp/bifrost-manifest-1.json example/basic/.bifrost/manifest.json
	rm -f /tmp/bifrost-manifest-1.json
