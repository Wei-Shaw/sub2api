# Backend 开发指南

**生成时间:** 2026-04-28  
**范围:** `backend/`

## 前置条件

- Go `1.26.2`，以 `backend/go.mod` 和 CI 为准。
- PostgreSQL，README 写 15+，Docker Compose 默认 18-alpine。
- Redis，README 写 7+，Docker Compose 默认 8-alpine。
- `golangci-lint`，CI 使用 action 版本 v2.9。
- 修改 DI 或 Ent schema 时需要 Wire 和 Ent generate 能正常运行。

## 常用命令

```bash
cd backend
go test ./...
make test-unit
make test-integration
make build
make generate
```

根目录入口：

```bash
make build-backend
make test-backend
make test
```

## 本地运行

1. 准备数据库、Redis 和配置文件。
2. 首次启动可走 setup wizard 或自动 setup。
3. 后端启动入口是：

```bash
cd backend
go run ./cmd/server
```

常用辅助：

```bash
cd backend
go run ./cmd/server --version
go run ./cmd/server --setup
```

## 代码生成

需要运行 `make generate` 的典型场景：

- 修改 `backend/ent/schema/*.go`
- 修改 `cmd/server/wire.go`
- 修改 `internal/*/wire.go`
- 新增/调整构造函数或 ProviderSet

生成输出包括 `backend/ent` 和 `backend/cmd/server/wire_gen.go`，提交前检查生成 diff 是否符合预期。

## 测试策略

- 单元/普通测试：`go test ./...`
- Unit build tag：`make test-unit`
- Integration build tag：`make test-integration`
- E2E 本地：`make test-e2e-local`

集成测试使用 testcontainers、PostgreSQL、Redis。涉及 repository、迁移、调度、计费、网关 failover、OAuth/token refresh、usage 统计时，应优先补充对应 package 的测试。

## 分层约束

- Handler 只处理 HTTP/DTO/响应，不承载核心业务。
- Service 负责业务流程，并通过接口依赖 repository/cache/upstream。
- Repository 负责 Ent/SQL/Redis/HTTP 上游访问。
- `backend/ent` 生成代码不手改。
- 新后端依赖需要加入对应层的 Wire ProviderSet。

## 高风险改动清单

- 网关流式响应、SSE、failover。
- 计费、余额、订阅限额、usage record worker。
- URL 安全、上游响应头过滤、CSP、trusted proxy、Turnstile。
- OAuth/token refresh，多平台平台语义不能互相套用。
- timezone 相关统计、今日边界、订阅过期。
- `RUN_MODE=simple` 和 backend mode 的前后端限制。

## CI

`.github/workflows/backend-ci.yml` 后端相关检查：

- setup-go 使用 `backend/go.mod`
- 显式校验 `go1.26.2`
- `make test-unit`
- `make test-integration`
- `golangci-lint run ./...`
