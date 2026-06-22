doctor:
	cd .. && go run ./cmd/bifrost doctor example/cmd/full

dev:
	cd .. && go run ./cmd/bifrost dev ./example/cmd/full/main.go

build:
	rm -rf cmd/full/.bifrost/*
	cd .. && go run ./cmd/bifrost build ./example/cmd/full/main.go
	go build -o bifrost ./cmd/full/main.go
	rm -rf cmd/full/.bifrost/*

start: build
	./bifrost
