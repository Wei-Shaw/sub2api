# Sub2API 项目上手指南（给 Vue + Spring Boot 开发者）

> 本文档面向熟悉 **Vue + Spring Boot** 的 Java 开发者，帮助快速理解本仓库（Vue + Go）的运行逻辑与代码组织，便于上手开发。
>
> 相关文档：[`DEV_GUIDE.md`](../DEV_GUIDE.md)（环境配置与常见坑）、[`AGENTS.md`](../AGENTS.md)（仓库规范）、[`README_CN.md`](../README_CN.md)（产品说明）。

---

## 项目是干什么的（业务心智模型）

**Sub2API** 是一个 **AI API 网关平台**：把上游 AI 账号（Claude / OpenAI / Gemini / Grok 等）的订阅配额，通过平台自建 API Key 分发给用户，并做鉴权、计费、负载均衡、故障切换。

可以把它想成：

| 你熟悉的 Spring 概念 | 本项目对应 |
|---|---|
| Spring Boot 应用 | Go + Gin 单体服务 |
| Controller | `handler/` |
| Service | `service/` |
| Repository / MyBatis / JPA | `repository/` + **Ent ORM** |
| `@Configuration` / `application.yml` | `config/` + `config.yaml` |
| Spring 依赖注入 | **Google Wire**（编译期 DI） |
| Filter / Interceptor | Gin **Middleware** |
| Flyway / Liquibase | `migrations/` SQL |
| Vue 管理后台 | `frontend/`（Vue3 + Pinia + Vite） |

### 请求与业务全景

```
客户端 (Claude Code / Codex / 自研 App)
        │  带 API Key 调用 /v1/messages 或 /v1/chat/completions
        ▼
   Sub2API 网关
        │  1. 校验 API Key / 余额 / 并发
        │  2. 选一个上游账号（粘性会话 + 负载）
        │  3. 转发到 Anthropic / OpenAI / Gemini ...
        │  4. 记 usage、扣费
        ▼
   上游 AI 服务

同时：
  管理员/用户 ──JWT──> /api/v1/... 管理后台、充值、API Key 管理
```

### 核心实体

对应 `backend/ent/schema/`：

| 实体 | 含义 |
|---|---|
| **User** | 平台用户（余额、角色 admin/user） |
| **APIKey** | 用户调用网关的钥匙 |
| **Group** | 分组（绑定平台类型、渠道、倍率等） |
| **Account** | 上游真实账号（OAuth / API Key 凭证） |
| **UsageLog** | 每次调用的用量与计费记录 |
| **Subscription / Redeem / Payment** | 订阅、卡密、支付 |

---

## 目录结构（对照 Spring 分层）

```
sub2api/
├── backend/                    # 后端（≈ 整个 Spring Boot 工程）
│   ├── cmd/server/             # main 入口 + Wire 组装（≈ Application 启动类）
│   ├── ent/schema/             # 实体定义（≈ JPA Entity）
│   ├── ent/                    # Ent 生成代码（别手改）
│   ├── migrations/             # SQL 迁移
│   ├── internal/
│   │   ├── config/             # 配置加载
│   │   ├── handler/            # HTTP 层（≈ Controller）
│   │   │   └── admin/          # 管理端接口
│   │   ├── service/            # 业务逻辑（≈ Service）
│   │   ├── repository/         # 数据访问 + Redis 缓存
│   │   ├── server/
│   │   │   ├── router.go       # 路由总装
│   │   │   ├── routes/         # 按模块注册路由
│   │   │   └── middleware/     # JWT / Admin / APIKey 鉴权等
│   │   ├── payment/            # 支付子系统
│   │   ├── securityaudit/      # Prompt 安全审计
│   │   ├── setup/              # 首次安装向导
│   │   └── web/                # 嵌入前端静态资源
│   └── config.yaml
│
├── frontend/                   # Vue3 SPA（管理台 + 用户台）
│   └── src/
│       ├── api/                # axios 封装（≈ request.js + 各模块 API）
│       ├── views/              # 页面
│       ├── components/
│       ├── stores/             # Pinia（≈ Vuex）
│       ├── router/             # 路由 + 守卫
│       └── i18n/
│
├── deploy/                     # Docker / 安装脚本
└── docs/                       # 文档（含本文）
```

### 和 Spring 最大的体感差异

1. **没有 class 的 IoC 容器运行时扫描**，而是 **Wire 在编译期** 把依赖画成图，生成 `wire_gen.go`。
2. **Handler 不写注解路由**，在 `routes/*.go` 里显式 `GET/POST` 绑定。
3. **Ent** 像类型安全的查询 DSL，改 schema 后要 `go generate` 再生代码。

---

## 启动流程（从 `main` 到监听端口）

入口：`backend/cmd/server/main.go`

```
main()
  ├─ -version          → 打印版本退出
  ├─ -setup            → CLI 安装向导
  ├─ NeedsSetup()?
  │    ├─ Docker 自动配置 → AutoSetupFromEnv
  │    └─ 否则 → runSetupServer()  (只开安装页)
  └─ runMainServer()
       ├─ 加载配置 + 日志
       ├─ initializeApplication()   // Wire 注入整棵依赖树
       ├─ PromptAudit.Start()       // 可选后台审计
       ├─ app.Server.ListenAndServe()
       └─ 等 SIGINT/SIGTERM → 优雅关闭
```

Wire 组装顺序（`cmd/server/wire.go` / `wire_gen.go`）大致是：

```
Config → Ent(PG) + Redis
  → Repository
  → Service（Auth / Gateway / Billing / Ops ...）
  → Middleware + Handler
  → Server（HTTP + 路由）
  → Application{Server, Cleanup}
```

**对应 Spring：**  
`Application.main` → `@SpringBootApplication` 扫包 → 创建 Bean → 内嵌 Tomcat 启动。  
这里是 **手写 `initializeApplication` + Wire 生成代码**，逻辑等价。

---

## HTTP 请求怎么走

### 1. 全局中间件（≈ Filter 链）

`backend/internal/server/router.go` 的 `SetupRouter`：

1. RequestLogger / SessionBinding / CORS / SecurityHeaders
2. 前端静态资源（生产把 Vue dist **嵌入二进制**）
3. 注册业务路由

### 2. 三类 API（先分清再改代码）

| 类型 | 路径前缀 | 鉴权 | 用途 |
|---|---|---|---|
| **面板 API** | `/api/v1/auth/*`、`/api/v1/user/*`、`/api/v1/admin/*` | JWT（Cookie/Bearer） | 登录、管理后台、充值 |
| **网关 API** | `/v1/messages`、`/v1/chat/completions`、`/v1/responses` 等 | **API Key** | 给外部 AI 客户端调用 |
| **支付回调** | 支付相关 webhook | 签名校验 | 第三方支付通知 |

路由注册在：

| 文件 | 职责 |
|---|---|
| `routes/auth.go` | 登录注册 OAuth |
| `routes/user.go` | 用户侧 |
| `routes/admin.go` | 管理侧 |
| `routes/gateway.go` | **核心网关** |
| `routes/payment.go` | 支付 |

### 3. 网关一次调用的完整链路（最重要）

以 `POST /v1/messages` 为例（`handler/gateway_handler.go`）：

```
客户端 ──Authorization: Bearer sk-xxx──►
  APIKeyAuth 中间件
    → Redis/DB 查 Key，写入 Context（user、group、subscription）
  GatewayHandler.Messages
    → 解析 body（model、stream）
    → 安全审计（可选）
    → 占用【用户并发槽】
    → 检查余额/订阅
    → 生成粘性 session hash，尽量绑同一上游账号
    → SelectAccount（选账号 + 占用【账号并发槽】）
    → Forward 到上游（可能 SSE 流式）
    → 失败则 Failover 换账号重试
    → 记 usage、扣费、释放槽位
```

**对应 Spring 写法的心智等价：**

```java
// Spring 大致等价
@RestController
public class GatewayController {
  @PostMapping("/v1/messages")
  public void messages(@RequestAttribute ApiKey key, ...) {
    // 中间件已经把 key/user 放进 request attribute
    concurrencyService.acquireUser(...);
    billingService.check(...);
    Account acc = gatewayService.selectAccount(...);
    gatewayService.forward(...);
  }
}
```

Go 里就是 `handler` 调 `service`，`service` 再调 `repository` / HTTP upstream。

---

## 前端运行逻辑

### 启动

`frontend/src/main.ts`：

1. 主题 / iOS viewport 修复
2. `createApp` + Pinia
3. `appStore.initFromInjectedConfig()`（站点名/logo 由后端注入，避免闪烁）
4. i18n → router → mount

### 路由

`frontend/src/router/index.ts`：懒加载页面 + 导航守卫

- `/setup` — 首次安装
- `/login` `/register` — 公开
- 用户区、管理区 — 需登录 / admin 角色

### 调后端

`frontend/src/api/client.ts`：axios 实例

- `baseURL` 指向后端
- 请求拦截器挂 `Authorization: Bearer <token>`
- 401 时自动 refresh token
- 管理接口带 admin UI 标记头

模块 API 按域拆分：`api/auth.ts`、`api/admin/users.ts`、`api/keys.ts` 等，和 Spring 按 Controller 分包类似。

### 开发时前后端关系

| 模式 | 说明 |
|---|---|
| **开发** | `pnpm dev`（Vite）+ 单独起 Go server；Vite 会代理/注入 public settings |
| **生产** | 前端 build 后 **嵌入 Go 二进制**，一个进程同时提供 API + 静态页 |

---

## 技术栈对照表

| 能力 | Spring Boot 世界 | Sub2API |
|---|---|---|
| Web 框架 | Spring MVC | **Gin** |
| ORM | JPA / MyBatis | **Ent** |
| DI | Spring IoC | **Wire**（编译期） |
| 配置 | application.yml | **config.yaml** + env |
| 缓存 | Redis + Spring Cache | Redis（手写 cache 层） |
| 认证面板 | Spring Security JWT | `JWTAuthMiddleware` |
| 认证网关 | 自定义 Filter | `APIKeyAuthMiddleware` |
| 迁移 | Flyway | `migrations/*.sql` |
| 前端 | Vue3 | Vue3 + Pinia + Tailwind + i18n |
| 包管理前端 | npm/yarn | **必须用 pnpm** |
| 数据库 | MySQL/PG | **PostgreSQL 15+** |
| 缓存/队列 | Redis | **Redis 7+** |

---

## 本地怎么跑起来

按根目录 [`DEV_GUIDE.md`](../DEV_GUIDE.md)：

**依赖：** PostgreSQL 16、Redis 7、Go 1.25.7、pnpm

```powershell
# 后端
cd backend
go run ./cmd/server

# 前端（另一个终端）
cd frontend
pnpm install
pnpm dev
```

首次无配置会进 **Setup 向导**（网页配置 DB/Redis/管理员）。  
Docker 可用 `deploy/` 下 compose。

### 常用命令

```bash
# 后端单测
cd backend && go test -tags=unit ./...

# 后端集成测试
cd backend && go test -tags=integration ./...

# 后端 lint
cd backend && golangci-lint run ./...

# 改了 ent/schema 后
cd backend && go generate ./ent

# 前端依赖（必须 pnpm）
cd frontend && pnpm install

# 前端 lint
cd frontend && pnpm run lint:check
```

Windows 无 `make` 时，直接使用 Makefile 里的原始命令即可。

---

## 上手改需求时怎么找代码

| 你要做的事 | 先看哪里 |
|---|---|
| 加/改管理后台接口 | `handler/admin/*` + `routes/admin.go` + 前端 `api/admin/*` + `views/` |
| 改登录/注册/OAuth | `handler/auth_*.go` + `routes/auth.go` + `views/auth/` |
| 改网关转发/计费/选账号 | `handler/gateway_*.go`、`openai_gateway_*.go` + `service/*gateway*`、`*billing*` |
| 改表结构 | `ent/schema/*.go` → generate → 必要时加 `migrations/` |
| 改中间件鉴权 | `server/middleware/` |
| 改系统配置项 | `config/`、`service` 的 Setting、`handler` setting |
| 支付 | `internal/payment/` + [`docs/PAYMENT_CN.md`](./PAYMENT_CN.md) |

### 建议阅读顺序（1～2 天）

1. `cmd/server/main.go` — 启动
2. `server/router.go` + `routes/*.go` — 全站 API 地图
3. `middleware/api_key_auth.go` + `handler/gateway_handler.go` 的 `Messages` — 核心链路
4. 随便挑一个简单 admin 接口（如 announcement）走通 handler → service → repository
5. 前端 `api/client.ts` + 对应 `views` 看面板怎么调

---

## 和 Spring Boot 开发时的注意点

1. **错误处理**：多靠返回 `error`，不是抛异常；HTTP 层用 `response.ErrorFrom(c, err)`。
2. **Context 贯穿**：超时/取消用 `context.Context`（类似传递 request scope）。
3. **interface + mock**：给 interface 加方法，**所有 test stub 都要补**，否则编译挂。
4. **不要手改 `ent/` 生成代码、`wire_gen.go`**；改 schema / 依赖后重新 generate。
5. **前端锁文件**：`package.json` 变了必须 `pnpm install` 并提交 `pnpm-lock.yaml`。
6. **网关是热点路径**：选账号、并发、failover、流式写回逻辑复杂，改前先读完整条链路再动。
7. **提交规范**：Conventional Commits（如 `feat(auth): ...`、`fix(ollama): ...`）。

---

## 一句话总结

> **Sub2API = 一个 Go 单体：内嵌 Vue 管理台 + 对外 AI 兼容网关。**  
> 面板走 JWT `/api/v1/*`，真正转发与计费的是 **API Key 网关**（鉴权 → 计费 → 选上游账号 → 转发 → 记用量）。  
> 分层和 Spring Boot 几乎一一对应，只是 DI 用 Wire、ORM 用 Ent、路由用 Gin 显式注册。

---

## 附录：关键路径速查

| 路径 | 说明 |
|---|---|
| `backend/cmd/server/main.go` | 进程入口 |
| `backend/cmd/server/wire.go` | DI 组装定义 |
| `backend/cmd/server/wire_gen.go` | Wire 生成代码（勿手改） |
| `backend/internal/server/router.go` | 路由与全局中间件 |
| `backend/internal/server/routes/` | 分模块路由注册 |
| `backend/internal/server/middleware/` | JWT / Admin / APIKey 等 |
| `backend/internal/handler/` | HTTP Handler |
| `backend/internal/service/` | 业务逻辑 |
| `backend/internal/repository/` | 数据访问与缓存 |
| `backend/ent/schema/` | 实体 Schema |
| `backend/config.yaml` | 配置样例 |
| `frontend/src/main.ts` | 前端入口 |
| `frontend/src/api/client.ts` | axios 与鉴权拦截 |
| `frontend/src/router/index.ts` | 前端路由 |
| `DEV_GUIDE.md` | 本地环境与坑点 |
| `AGENTS.md` | 构建/测试/提交规范 |
