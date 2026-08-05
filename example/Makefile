doctor:
	cd .. && go run ./cmd/bifrost doctor ./example/cmd/full

dev:
	cd .. && go run ./cmd/bifrost dev ./example/cmd/full/main.go

build:
	cd .. && go run ./cmd/bifrost build ./example/cmd/full/main.go --go-build=./bifrost

start: build
	./bifrost
