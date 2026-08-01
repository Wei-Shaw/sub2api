.PHONY: build generate test test-unit test-integration test-usage-billing-integration test-e2e

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

test-usage-billing-integration:
	@test -n "$${SUB2API_TEST_POSTGRES_DSN}" || (echo "SUB2API_TEST_POSTGRES_DSN is required" >&2; exit 1)
	@test -n "$${SUB2API_TEST_REDIS_ADDR}" || (echo "SUB2API_TEST_REDIS_ADDR is required" >&2; exit 1)
	go test -tags=integration ./cmd/server -run '^TestGatewayHTTPUsageBilling_DurableSettlementAndUpstreamFailureBoundary$$' -count=1 -timeout=180s
	go test -tags=integration ./internal/repository -run 'TestUsageBilling(Settlement|SettlementWorker)' -count=1 -timeout=120s

test-e2e:
	./scripts/e2e-test.sh

test-e2e-local:
	go test -tags=e2e -v -timeout=300s ./internal/integration/...
