.PHONY: doctor dev build start dev-quickjs build-quickjs start-quickjs

doctor:
	cd .. && go run ./cmd/bifrost doctor ./example/cmd/full

dev:
	cd .. && go run ./cmd/bifrost dev ./example/cmd/full/main.go

build:
	cd .. && go run ./cmd/bifrost build ./example/cmd/full/main.go --go-build=./bifrost

start: build
	BIFROST_QUICKJS_WORKERS=8 ./bifrost

dev-quickjs: dev

build-quickjs:
	cd .. && go run ./cmd/bifrost build ./example/cmd/full/main.go --go-build=./bifrost-quickjs

start-quickjs: build-quickjs
	BIFROST_QUICKJS_WORKERS=8 ./bifrost-quickjs
