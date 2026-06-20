dev:
	go run ../cmd/bifrost dev main.go

build:
	go run ../cmd/bifrost build main.go
	go build -o ./tmp/app .

start: build
	./tmp/app

doctor:
	go run github.com/3-lines-studio/bifrost/cmd/bifrost@latest doctor .
