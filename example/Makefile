init:
	go run ../cmd/init .

start: build
	./bifrost

dev-full:
	BIFROST_DEV=1 air -c .air.toml --build.cmd "go build -o ./tmp/main ./cmd/full/main.go"

build-full:
	go run ../cmd/build ./cmd/full/main.go
	go build -o bifrost ./cmd/full/main.go

dev-static:
	BIFROST_DEV=1 air -c .air.toml --build.cmd "go build -o ./tmp/main ./cmd/static/main.go"

build-static:
	go run ../cmd/build ./cmd/static/main.go
	go build -o bifrost ./cmd/static/main.go

dev-desktop:
	BIFROST_DEV=1 air -c .air.toml --build.cmd "go build -o ./tmp/main ./cmd/desktop/main.go"

build-desktop:
	go run ../cmd/build ./cmd/desktop/main.go
	go build -o bifrost-desktop ./cmd/desktop/main.go

start-desktop: build-desktop
	./bifrost-desktop
