dev:
	go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest dev ./main.go

build:
	go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest build ./main.go --go-build

start: build
	./tmp/app

doctor:
	go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest doctor .
