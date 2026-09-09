PLUGIN_ID := com.cw.remind
VERSION := 0.1.1

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
	rm -rf dist/$(PLUGIN_ID)
	mkdir -p dist/$(PLUGIN_ID)/server/dist dist/$(PLUGIN_ID)/webapp/dist dist/$(PLUGIN_ID)/assets dist/$(PLUGIN_ID)/public
	cp plugin.json dist/$(PLUGIN_ID)/plugin.json
	cp -R server/dist/. dist/$(PLUGIN_ID)/server/dist/
	cp -R webapp/dist/. dist/$(PLUGIN_ID)/webapp/dist/
	cp -R assets/. dist/$(PLUGIN_ID)/assets/
	cp assets/duewatch-icon.png dist/$(PLUGIN_ID)/public/duewatch-icon.png
	tar -czf dist/$(PLUGIN_ID)-$(VERSION).tar.gz -C dist $(PLUGIN_ID)
	rm -rf dist/$(PLUGIN_ID)

test:
	go test ./server/...
	cd webapp && npm run check-types

clean:
	rm -rf dist server/dist webapp/dist

docker-build:
	docker compose run --rm builder
