doctor:
	cd .. && go run ./cmd/bifrost doctor example

dev:
	cd .. && go run ./cmd/bifrost dev ./example/cmd/full/main.go

build:
	rm -rf .bifrost/*
	cd .. && go run ./cmd/bifrost build ./example/cmd/full/main.go
	go build -o bifrost ./cmd/full/main.go
	rm -rf .bifrost/*

start: build
	./bifrost
