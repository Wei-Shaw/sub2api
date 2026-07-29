.PHONY: build generate test test-unit test-integration test-e2e test-e2e-codebuddy-db

VERSION ?= $(shell ./scripts/resolve-version.sh)
LDFLAGS ?= -s -w -X main.Version=$(VERSION)

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/server ./cmd/server

generate:
	go generate ./ent
	go generate ./cmd/server

test:
	go test ./...
	golangci-lint run ./...

test-unit:
	go test -tags=unit ./...

test-integration:
	go test -tags=integration ./...

test-e2e:
	./scripts/e2e-test.sh

test-e2e-local:
	go test -tags=e2e -v -timeout=300s ./internal/integration/...

# Reads an active CodeBuddy API key/account/model from PostgreSQL and validates
# /v1/messages, /v1/chat/completions, and /v1/responses in stream/non-stream mode.
# Required: CODEBUDDY_E2E_DATABASE_DSN. Optional: BASE_URL, API key/group/model IDs.
test-e2e-codebuddy-db:
	CODEBUDDY_DB_E2E=1 go test -tags=e2e -v -timeout=300s -run '^TestCodeBuddyDatabaseBackedIngressMatrix$$' ./internal/integration/...
