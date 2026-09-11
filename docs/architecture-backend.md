# Backend 架构文档

**生成时间:** 2026-04-28  
**扫描级别:** Quick Scan  
**范围:** `backend/`

## 概览

后端是 Sub2API 的主服务进程，负责用户/管理员 API、AI 网关转发、鉴权、计费、调度、支付、运维监控、数据库迁移和内嵌前端静态资源服务。Quick Scan 依据 `go.mod`、`cmd/server/main.go`、`internal/server/router.go`、`internal/server/routes/*`、Ent schema 文件和目录结构生成，没有逐个业务函数审计。

## 技术栈

| 类别 | 技术 | 版本/来源 | 用途 |
| --- | --- | --- | --- |
| 语言 | Go | `backend/go.mod`: `1.26.2` | 后端主语言 |
| HTTP 框架 | Gin | `1.9.1` | 路由、中间件、HTTP handler |
| ORM | Ent | `0.14.5` | schema、查询、迁移辅助 |
| DI | Wire | `0.7.0` | 编译期依赖注入 |
| 数据库 | PostgreSQL | README 15+，Dockerfile 使用 18-alpine 客户端 | 持久化 |
| 缓存/队列 | Redis | README 7+ | 速率限制、缓存、状态 |
| 日志 | zap/slog bridge | `go.uber.org/zap` + internal logger | 结构化日志 |
| 支付 | Alipay/WeChat Pay/Stripe/EasyPay | `internal/payment` | 内置充值与订阅购买 |

## 入口与生命周期

- `backend/cmd/server/main.go` 是主入口，支持 `--setup`、`--version`、首次启动 setup wizard、自动 setup 和正常服务模式。
- `backend/cmd/server/wire.go` 与 `wire_gen.go` 负责组合服务依赖，修改 ProviderSet 后需要重新生成。
- `backend/internal/server/router.go` 注册中间件、内嵌前端、公共路由、`/api/v1` 业务路由和网关路由。
- `backend/internal/server/http.go` 负责 HTTP server 组装。
- `backend/internal/setup` 提供首次配置流程。

## 分层结构

| 层 | 目录 | 职责 |
| --- | --- | --- |
| Handler | `backend/internal/handler` | HTTP 请求解析、DTO 映射、响应封装、网关协议入口 |
| Routes | `backend/internal/server/routes` | Gin 路由表和认证/权限中间件挂载 |
| Service | `backend/internal/service` | 业务流程、调度、计费、OAuth、支付、运维监控、网关转发逻辑 |
| Repository | `backend/internal/repository` | Ent、SQL、Redis、上游 HTTP、缓存、迁移执行 |
| Domain/Model | `backend/internal/domain`, `backend/internal/model` | 领域常量、轻量模型和共享语义 |
| Infra Packages | `backend/internal/pkg`, `backend/internal/util` | OAuth provider、API 兼容转换、HTTP client、TLS 指纹、日志、时间区、安全工具 |
| Persistence | `backend/ent/schema`, `backend/migrations` | 数据模型源和 SQL 迁移 |
| Web Embed | `backend/internal/web` | 内嵌前端资源、HTML 缓存、公共设置注入 |

项目上下文明确要求保持 `handler -> service -> repository` 边界，避免 handler 或多数 service 直接绕过接口访问底层存储。

## 路由架构

`registerRoutes` 将路由分成五组：

- `RegisterCommonRoutes(r)`：`/health`、`/setup/status`、Claude Code 事件日志兼容端点。
- `RegisterAuthRoutes(v1, ...)`：`/api/v1/auth/*`、OAuth、token refresh、公开设置、当前用户会话。
- `RegisterUserRoutes(v1, ...)`：登录用户的 profile、API key、usage、redeem、subscription、channel monitor 等接口。
- `RegisterAdminRoutes(v1, ...)`：管理员的 dashboard、user、group、account、ops、settings、payment、channel 等接口。
- `RegisterGatewayRoutes(r, ...)`：不挂在 `/api/v1` 下的 AI 网关兼容接口，如 `/v1/messages`、`/v1/responses`、`/v1beta/models`。

## 网关兼容层

网关路由集中在 `backend/internal/server/routes/gateway.go`，规范化端点定义在 `backend/internal/handler/endpoint.go`。

核心端点：

- Claude/Anthropic 兼容：`POST /v1/messages`、`POST /v1/messages/count_tokens`、`GET /v1/models`、`GET /v1/usage`
- OpenAI 兼容：`POST /v1/responses`、`POST /v1/chat/completions`、`POST /v1/images/generations`、`POST /v1/images/edits`
- Responses 直连别名：`/responses`、`/backend-api/codex/responses`
- Gemini 兼容：`GET /v1beta/models`、`GET /v1beta/models/:model`、`POST /v1beta/models/*modelAction`
- Antigravity 专用：`/antigravity/v1/*`、`/antigravity/v1beta/*`

关键中间件包括请求体大小限制、client request id、运维错误日志、端点规范化、API key 鉴权、分组校验和平台强制路由。

## 配置与安全边界

- 配置加载集中在 `backend/internal/config`，部署样例位于 `deploy/config.example.yaml` 和 `deploy/.env.example`。
- HTTP 响应应通过 `backend/internal/pkg/response` 统一外壳返回，前端 Axios client 会自动解包 `{ code, message, data }`。
- 鉴权包括 JWT、API key、admin auth、OAuth、TOTP、backend mode/simple mode、Turnstile、rate limiter 等。
- URL 安全、CSP、上游响应头过滤、请求体限制、SSE failover、敏感日志脱敏是高风险边界。

## 数据访问

- Ent schema 源文件在 `backend/ent/schema`，生成文件在 `backend/ent`，不应手改生成文件。
- SQL 迁移在 `backend/migrations`，`migrations.go` 和 repository 迁移 runner 负责执行/校验。
- Redis 用于 token/cache/rate limit/concurrency/session/scheduler 等运行态数据。

## 后台任务与异步处理

从文件名可见的长期或异步能力包括：

- token refresh、quota fetch、scheduled test runner、subscription maintenance、usage record worker pool
- ops metrics collector、alert evaluator、cleanup service、scheduled report、system log sink
- scheduler outbox、timing wheel、gateway/identity/billing/concurrency cache

这些服务通常通过 Wire provider 注入，并需要明确 Start/Stop 生命周期。

## 测试策略

- 后端入口：`cd backend && go test ./...`
- Make 目标：`make test-unit`、`make test-integration`、`make test-e2e-local`
- 集成测试使用 `//go:build integration` 和 testcontainers；本地 Docker 不可用时部分测试可能跳过，CI 中应失败。
- `.github/workflows/backend-ci.yml` 会校验 Go `1.26.2`、运行 unit/integration、执行 golangci-lint。

## 修改注意事项

- Ent schema 改动后运行 `cd backend && make generate`。
- Wire ProviderSet 或构造函数变化后也需要重新生成 Wire。
- 网关流式响应一旦向客户端写入 SSE 内容，不应再拼接第二个上游流。
- 计费、余额、usage record、billing cache 失败时应 fail-closed。
- 涉及日期范围、今日统计、订阅过期和日志聚合时显式处理 timezone。
