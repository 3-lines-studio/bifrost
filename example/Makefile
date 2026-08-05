.PHONY: doctor dev build start dev-sobek build-sobek start-sobek

doctor:
	cd .. && go run ./cmd/bifrost doctor ./example/cmd/full

dev:
	cd .. && go run ./cmd/bifrost dev ./example/cmd/full/main.go

build:
	cd .. && go run ./cmd/bifrost build ./example/cmd/full/main.go --go-build=./bifrost

start: build
	./bifrost

dev-sobek:
	cd .. && BIFROST_JS_RUNTIME=sobek BIFROST_SOBEK_WORKERS=4 go run ./cmd/bifrost dev ./example/cmd/full/main.go

build-sobek:
	cd .. && BIFROST_JS_RUNTIME=sobek BIFROST_SOBEK_WORKERS=4 go run ./cmd/bifrost build ./example/cmd/full/main.go --go-build=./bifrost-sobek

start-sobek: build-sobek
	BIFROST_SOBEK_WORKERS=4 ./bifrost-sobek
