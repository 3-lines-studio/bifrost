APP := .

.PHONY: build css css-watch dev serve check

build:
	go run ../../cmd/bifrost build $(APP)

css:
	bunx @tailwindcss/cli@4.3.3 -i assets/tailwind.css -o public/styles.css --minify

css-watch:
	bunx @tailwindcss/cli@4.3.3 -i assets/tailwind.css -o public/styles.css --watch

dev:
	go run ../../cmd/bifrost dev $(APP)

serve: build
	BIFROST_ADDR=127.0.0.1:18900 ./.bifrost/bifrost-app

check: build
	bash scripts/check.sh
