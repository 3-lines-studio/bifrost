doctor:
	go run ../cmd/bifrost doctor .

dev:
	go run ../cmd/bifrost dev ./cmd/full/main.go

build:
	rm -rf .bifrost/*
	go run ../cmd/bifrost build ./cmd/full/main.go
	go build -o bifrost ./cmd/full/main.go
	rm -rf .bifrost/*

start: build
	./bifrost
