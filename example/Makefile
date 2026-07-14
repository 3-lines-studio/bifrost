doctor:
	cd .. && go run ./cmd/bifrost doctor example

dev:
	cd .. && go run ./cmd/bifrost dev ./example/cmd/full/main.go

build:
	mkdir -p .bifrost && touch .bifrost/.gitkeep
	cd .. && go run ./cmd/bifrost build ./example/cmd/full/main.go
	rm -rf .bifrost && cp -r cmd/full/.bifrost . && echo '# This file ensures .bifrost directory exists for go:embed' > .bifrost/.gitkeep
	cd cmd/full && go build -o ../../bifrost .
	rm -rf cmd/full/.bifrost

start: build
	./bifrost
