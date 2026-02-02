.PHONY: dev build run

init:
	go run ../cmd/init .

dev:
	BIFROST_DEV=1 air --build.exclude_dir "node_modules,.bifrost"


build:
	rm -rf .bifrost/*
	rm -f bifrost
	go run ../cmd/build main.go
	go build -o bifrost main.go

run: build
	rm -rf .bifrost/*
	./bifrost

