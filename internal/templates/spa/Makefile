dev:
	bifrost dev ./main.go

build:
	bifrost build ./main.go --go-build

start: build
	./tmp/app

doctor:
	bifrost doctor .
