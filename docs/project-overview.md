# sub2api 项目总览

**生成时间:** 2026-04-28  
**类型:** 多部分 Web/API 网关项目  
**扫描级别:** Quick Scan

## 执行摘要

Sub2API 是面向 AI 产品订阅额度分发的 API 网关平台。它通过后端服务管理用户、分组、上游账号、API key、订阅、计费、支付、调度和运维监控；通过前端 SPA 提供用户控制台与管理员后台；通过网关兼容层向外暴露 Claude/OpenAI/Gemini/Antigravity/Codex 风格接口。

本次文档为 AI 辅助开发上下文而生成，基于 README、配置、构建文件、路由注册、Ent schema、目录结构和现有文档快速扫描。若后续要进行字段级 API 合约、数据库约束或复杂业务流程改动，应补充 deep/exhaustive scan。

## 项目分类

- **Repository Type:** multi-part
- **Part 1:** `backend`，Go 后端/API 网关
- **Part 2:** `frontend`，Vue/Vite Web 前端
- **Primary Languages:** Go、TypeScript
- **Architecture Pattern:** 前后端同仓，多阶段构建，后端可内嵌前端资源；后端按 handler/service/repository 分层。

## 技术栈摘要

### Backend

| 类别 | 技术 |
| --- | --- |
| Runtime | Go 1.26.2 |
| HTTP | Gin 1.9.1 |
| ORM | Ent 0.14.5 |
| DI | Wire 0.7.0 |
| DB | PostgreSQL |
| Cache/Queue | Redis |
| Payments | EasyPay、Alipay、WeChat Pay、Stripe |

### Frontend

| 类别 | 技术 |
| --- | --- |
| Framework | Vue 3.5.26 |
| Build | Vite 5.4.21 |
| Language | TypeScript 5.6.3 |
| State | Pinia 2.3.1 |
| Router | Vue Router 4.6.4 |
| Styling | TailwindCSS 3.4.19 |
| Tests | Vitest 2.1.9 |

## 主要能力

- AI 网关：Anthropic Messages、OpenAI Responses/Chat Completions/Images、Gemini v1beta、Antigravity、Codex direct aliases。
- 账号调度：多上游账号、sticky session、并发控制、速率限制、临时不可调度、模型路由。
- SaaS 管理：用户、分组、API key、订阅、余额、卡密、优惠码、公告。
- 支付：内置充值和订阅购买，支持多支付 provider、订单、退款、webhook。
- 运维监控：实时并发、请求详情、错误日志、上游错误、告警、系统日志、聚合 dashboard。
- 安全：JWT、refresh token、OAuth、TOTP、Turnstile、CSP、URL 安全、响应头过滤、日志脱敏。
- 部署：一键安装、Docker Compose、GoReleaser、内嵌前端单二进制。

## 关键入口

- 后端主入口：`backend/cmd/server/main.go`
- 后端路由入口：`backend/internal/server/router.go`
- 后端网关路由：`backend/internal/server/routes/gateway.go`
- 数据模型源：`backend/ent/schema`
- 前端入口：`frontend/src/main.ts`
- 前端路由：`frontend/src/router/index.ts`
- 前端 API client：`frontend/src/api/client.ts`
- 前端构建配置：`frontend/vite.config.ts`
- 生产 Dockerfile：`Dockerfile`

## 构建与测试

```bash
# backend
cd backend
go test ./...
make test-unit
make test-integration
make build
make generate

# frontend
cd frontend
pnpm install --frozen-lockfile
pnpm run typecheck
pnpm run test:run
pnpm run build

# root
make test
make build
```

## 文档地图

- [项目文档索引](./index.md)
- [后端架构](./architecture-backend.md)
- [前端架构](./architecture-frontend.md)
- [集成架构](./integration-architecture.md)
- [API 合约目录](./api-contracts-backend.md)
- [数据模型目录](./data-models-backend.md)
- [组件清单](./component-inventory-frontend.md)
- [源码树分析](./source-tree-analysis.md)
- [后端开发指南](./development-guide-backend.md)
- [前端开发指南](./development-guide-frontend.md)
- [部署指南](./deployment-guide.md)

## 已知限制

- 本次为 Quick Scan，未逐源文件审计全部 handler/service/repository 实现。
- API 合约是路由级目录，不是字段级 OpenAPI。
- 数据模型未提取完整字段类型、索引、唯一约束、默认值和 migration 语义。
- `_bmad-output/project-context.md` 和部分中文注释/README 显示疑似编码损坏，涉及中文文档改动时应先确认编码。
