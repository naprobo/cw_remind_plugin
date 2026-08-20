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
rm -f dist/com.cw.remind-0.1.0.tar.gz
tar -czf dist/com.cw.remind-0.1.0.tar.gz plugin.json server/dist webapp/dist

echo "Built dist/com.cw.remind-0.1.0.tar.gz"
