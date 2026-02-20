doctor:
	go run ../cmd/doctor .

dev:
	@BIFROST_DEV=1 air -c .air.toml --build.cmd "go build -o ./tmp/main ./cmd/full/main.go"

build:
	rm -rf .bifrost/*
	go run ../cmd/build ./cmd/full/main.go
	go build -o bifrost ./cmd/full/main.go
	rm -rf .bifrost/*

start: build
	./bifrost

dev-static:
	BIFROST_DEV=1 air -c .air.toml --build.cmd "go build -o ./tmp/main ./cmd/static/main.go"

build-static:
	rm -rf .bifrost/*
	go run ../cmd/build ./cmd/static/main.go
	go build -o bifrost-static ./cmd/static/main.go
	rm -rf .bifrost/*

start-static: build-static
	./bifrost-static

dev-desktop:
	BIFROST_DEV=1 air -c .air.toml --build.cmd "go build -o ./tmp/main ./cmd/desktop/main.go"

build-desktop:
	rm -rf .bifrost/*
	go run ../cmd/build ./cmd/desktop/main.go
	go build -o bifrost ./cmd/desktop/main.go
	rm -rf .bifrost/*

start-desktop: build-desktop
	./bifrost
