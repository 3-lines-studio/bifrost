APP := .

.PHONY: build dev serve check browser

build:
	go run ../../cmd/bifrost build $(APP)

dev:
	go run ../../cmd/bifrost dev $(APP)

serve: build
	BIFROST_ADDR=127.0.0.1:18800 ./.bifrost/bifrost-app

check: build
	bash scripts/check.sh

browser: build
	bash scripts/browser.sh
