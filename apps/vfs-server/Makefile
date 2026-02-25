.PHONY: build run dev test

build:
	go build -o bin/vfs-server ./cmd/vfs-server

run: build
	./bin/vfs-server

dev:
	go run ./cmd/vfs-server

test:
	go test ./... -race -v
