APP := .

.PHONY: build dev serve check clean

build:
	go run ../../cmd/bifrost build $(APP)

dev:
	go run ../../cmd/bifrost dev $(APP)

serve: build
	BIFROST_ADDR=127.0.0.1:18700 ./.bifrost/bifrost-app

check: build
	bash scripts/check.sh

clean:
	rm -rf .bifrost
