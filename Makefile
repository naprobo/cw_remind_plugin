PLUGIN_ID := com.cw.remind
VERSION := 0.1.0

.PHONY: build server webapp dist clean test docker-build
build: server webapp

server:
	mkdir -p server/dist
	GOOS=linux GOARCH=amd64 go build -o server/dist/plugin-linux-amd64 ./server
	GOOS=linux GOARCH=arm64 go build -o server/dist/plugin-linux-arm64 ./server
	GOOS=darwin GOARCH=amd64 go build -o server/dist/plugin-darwin-amd64 ./server
	GOOS=darwin GOARCH=arm64 go build -o server/dist/plugin-darwin-arm64 ./server
	GOOS=windows GOARCH=amd64 go build -o server/dist/plugin-windows-amd64.exe ./server

webapp:
	cd webapp && npm install && npm run build

dist: build
	mkdir -p dist
	tar -czf dist/$(PLUGIN_ID)-$(VERSION).tar.gz plugin.json server/dist webapp/dist

test:
	go test ./server/...
	cd webapp && npm run check-types

clean:
	rm -rf dist server/dist webapp/dist

docker-build:
	docker compose run --rm builder
