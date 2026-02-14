.PHONY: dev build run

init:
	go run ../cmd/init .

dev:
	BIFROST_DEV=1 air -c .air.toml


build:
	go run ../cmd/build main.go
	go build -o bifrost main.go

start: build
	./bifrost

