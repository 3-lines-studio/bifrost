APP := .

.PHONY: build browser dev serve check

build:
	go run ../../cmd/bifrost build $(APP)

browser: build
	bash scripts/browser.sh

dev:
	go run ../../cmd/bifrost dev $(APP)

serve: build
	BIFROST_ADDR=127.0.0.1:18930 ./.bifrost/bifrost-app

check: build
	bash scripts/check.sh
