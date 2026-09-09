#!/bin/sh
set -eu

cd /workspace

echo "Formatting and testing Go server..."
gofmt -w server/*.go
go mod tidy
go test ./server/...

echo "Building server binaries..."
mkdir -p server/dist
GOOS=linux GOARCH=amd64 go build -trimpath -o server/dist/plugin-linux-amd64 ./server
GOOS=linux GOARCH=arm64 go build -trimpath -o server/dist/plugin-linux-arm64 ./server
GOOS=darwin GOARCH=amd64 go build -trimpath -o server/dist/plugin-darwin-amd64 ./server
GOOS=darwin GOARCH=arm64 go build -trimpath -o server/dist/plugin-darwin-arm64 ./server
GOOS=windows GOARCH=amd64 go build -trimpath -o server/dist/plugin-windows-amd64.exe ./server

echo "Checking and building web app..."
cd /workspace/webapp
npm install
npm run check-types
npm run build

echo "Packaging plugin..."
cd /workspace
mkdir -p dist
rm -f dist/com.cw.remind-0.1.1.tar.gz
rm -rf dist/com.cw.remind
mkdir -p dist/com.cw.remind/server/dist dist/com.cw.remind/webapp/dist dist/com.cw.remind/assets dist/com.cw.remind/public
cp plugin.json dist/com.cw.remind/plugin.json
cp -R server/dist/. dist/com.cw.remind/server/dist/
cp -R webapp/dist/. dist/com.cw.remind/webapp/dist/
cp -R assets/. dist/com.cw.remind/assets/
cp assets/duewatch-icon.png dist/com.cw.remind/public/duewatch-icon.png
tar -czf dist/com.cw.remind-0.1.1.tar.gz -C dist com.cw.remind
rm -rf dist/com.cw.remind

echo "Built dist/com.cw.remind-0.1.1.tar.gz"
