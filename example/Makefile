.PHONY: doctor dev build start dev-sobek build-sobek start-sobek dev-bun build-bun start-bun

doctor:
	cd .. && go run ./cmd/bifrost doctor ./example/cmd/full

dev:
	cd .. && BIFROST_SOBEK_WORKERS=4 go run ./cmd/bifrost dev ./example/cmd/full/main.go

build:
	cd .. && BIFROST_SOBEK_WORKERS=4 go run ./cmd/bifrost build ./example/cmd/full/main.go --go-build=./bifrost

start: build
	BIFROST_SOBEK_WORKERS=4 ./bifrost

dev-sobek: dev

build-sobek:
	cd .. && BIFROST_SOBEK_WORKERS=4 go run ./cmd/bifrost build ./example/cmd/full/main.go --go-build=./bifrost-sobek

start-sobek: build-sobek
	BIFROST_SOBEK_WORKERS=4 ./bifrost-sobek

dev-bun:
	cd .. && BIFROST_JS_RUNTIME=bun go run ./cmd/bifrost dev ./example/cmd/full/main.go

build-bun:
	cd .. && BIFROST_JS_RUNTIME=bun go run ./cmd/bifrost build ./example/cmd/full/main.go --go-build=./bifrost-bun

start-bun: build-bun
	BIFROST_JS_RUNTIME=bun ./bifrost-bun
