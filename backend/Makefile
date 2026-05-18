.PHONY: build generate test test-unit test-integration test-e2e prepare-seo

VERSION ?= $(shell tr -d '\r\n' < ./cmd/server/VERSION)
LDFLAGS ?= -s -w -X main.Version=$(VERSION)

SHARED_SEO_DIR = ../shared/seo
SEO_DATA_DIR = ./internal/web/seo/data

prepare-seo:
	@mkdir -p $(SEO_DATA_DIR)
	@cp $(SHARED_SEO_DIR)/seo.json $(SEO_DATA_DIR)/seo.json
	@cp $(SHARED_SEO_DIR)/faq.zh.json $(SEO_DATA_DIR)/faq.zh.json
	@cp $(SHARED_SEO_DIR)/faq.en.json $(SEO_DATA_DIR)/faq.en.json
	@echo "prepare-seo: copied 3 JSON files to $(SEO_DATA_DIR)/"

build: prepare-seo
	CGO_ENABLED=0 go build -tags=embed -ldflags="$(LDFLAGS)" -trimpath -o bin/server ./cmd/server

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
