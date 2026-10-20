APP := .

.PHONY: build dev serve check

build:
	go run ../../cmd/bifrost build $(APP)

dev:
	go run ../../cmd/bifrost dev $(APP)

serve: build
	BIFROST_ADDR=127.0.0.1:18900 ./.bifrost/bifrost-app

check: build
	bash scripts/check.sh
