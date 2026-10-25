.PHONY: build generate test test-unit test-integration test-integration-mysql test-e2e

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

# 在外部 MySQL 测试库上运行集成测试（会清空目标库！）。
# 需要环境变量 SUB2API_TEST_MYSQL_DSN，例如：
#   make test-integration-mysql SUB2API_TEST_MYSQL_DSN='user:pass@tcp(localhost:3306)/sub2api_test?parseTime=true'
# binlog 开启且账号无 SUPER 权限时默认跳过触发器 DDL（容错模式）；
# 设 SUB2API_TEST_MYSQL_STRICT_TRIGGERS=true 严格验证触发器（需 DBA 预开权限）。
test-integration-mysql:
	SUB2API_TEST_DB=mysql go test -tags=integration -timeout=25m ./internal/repository/

test-e2e:
	./scripts/e2e-test.sh

test-e2e-local:
	go test -tags=e2e -v -timeout=300s ./internal/integration/...
