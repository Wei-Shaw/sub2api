.PHONY: build test test-unit test-integration test-e2e

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...
	golangci-lint run ./...

test-unit:
	go test -tags=unit ./...

test-integration:
	go test -tags=integration ./...

test-e2e:
	go test -tags=e2e ./...
