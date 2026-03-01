doctor:
	go run ../cmd/doctor .

dev:
	@BIFROST_DEV=1 air -c .air.toml --build.cmd "go build -o ./tmp/main ./cmd/full/main.go"

build:
	rm -rf .bifrost/*
	go run ../cmd/build --framework svelte ./cmd/full/main.go
	go build -o bifrost ./cmd/full/main.go
	rm -rf .bifrost/*

start: build
	./bifrost
