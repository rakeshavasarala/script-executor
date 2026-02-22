.PHONY: help build run test lint generate docker-build

help:
	@echo "Script Executor - Makefile targets"
	@echo "  build        - Build the binary"
	@echo "  run          - Run locally"
	@echo "  test         - Run tests"
	@echo "  lint         - Run linter"
	@echo "  generate     - Regenerate proto code"
	@echo "  docker-build - Build Docker image"

build:
	go build -o bin/script-executor ./cmd/script-executor

run: build
	./bin/script-executor

test:
	go test ./...

lint:
	golangci-lint run ./...

generate:
	buf generate

docker-build:
	docker build -t script-executor:latest .
