# Source Tree Analysis

**生成时间:** 2026-04-28  
**扫描级别:** Quick Scan

## 概览

仓库是多部分项目：Go 后端、Vue 前端、部署资产、现有文档、BMad/agent 配置共存。Quick Scan 只展开关键目录和职责，不列出全部 2000+ 跟踪文件。

## 顶层结构

```text
sub2api/
|-- backend/                    # Go 后端、网关、数据模型、迁移、内嵌 web 资源
|   |-- cmd/
|   |   |-- server/              # 主服务入口、版本、Wire 生成入口
|   |   `-- jwtgen/              # JWT 辅助工具
|   |-- ent/
|   |   `-- schema/              # Ent schema 源文件；ent 其余目录为生成代码
|   |-- internal/
|   |   |-- config/              # 配置加载
|   |   |-- handler/             # HTTP handler、DTO、网关协议 handler
|   |   |-- middleware/          # 通用限流等中间件
|   |   |-- payment/             # 支付 provider 和金额/负载均衡工具
|   |   |-- pkg/                 # OAuth、API 兼容转换、HTTP client、日志、TLS 等基础包
|   |   |-- repository/          # Ent/SQL/Redis/HTTP 上游访问
|   |   |-- server/              # HTTP server、路由、中间件
|   |   |-- service/             # 业务服务、调度、计费、运维、网关
|   |   |-- setup/               # 首次安装向导
|   |   |-- util/                # 辅助工具
|   |   `-- web/                 # 前端内嵌资源服务
|   |-- migrations/              # SQL 迁移
|   `-- resources/               # 模型价格等运行资源
|-- frontend/                    # Vue/Vite/TypeScript SPA
|   |-- public/                  # 静态公开资源
|   |-- src/
|   |   |-- api/                 # Axios API 模块
|   |   |-- assets/              # 前端资产
|   |   |-- components/          # 通用/领域组件
|   |   |-- composables/         # Vue 组合式工具
|   |   |-- constants/           # 前端常量
|   |   |-- i18n/                # 语言包
|   |   |-- router/              # 路由和标题
|   |   |-- stores/              # Pinia stores
|   |   |-- styles/              # 样式补充
|   |   |-- types/               # 前端类型
|   |   |-- utils/               # 前端工具
|   |   `-- views/               # 页面视图
|   `-- vite.config.ts           # 构建、代理、chunk、public settings 注入
|-- deploy/                      # Docker Compose、安装脚本、生产配置样例、Caddy
|-- docs/                        # 项目文档和本次生成的 AI 上下文文档
|-- assets/                      # README/赞助商等静态资产
|-- tools/                       # 仓库辅助脚本
|-- .github/workflows/           # CI、release、安全扫描
|-- _bmad/                       # BMad 配置和脚本
|-- _bmad-output/                # BMad 输出上下文
|-- .agents/skills/              # 本地 agent skills
|-- Dockerfile                   # 生产多阶段镜像构建
|-- Makefile                     # 根构建/测试入口
|-- README.md                    # 英文 README
|-- README_CN.md                 # 中文 README；当前文件疑似编码损坏
|-- README_JA.md                 # 日文 README
`-- DEV_GUIDE.md                 # 开发指南
```

## 后端关键目录

| 路径 | 说明 |
| --- | --- |
| `backend/cmd/server/main.go` | 服务入口、setup mode、自动 setup、主 server 生命周期 |
| `backend/internal/server/router.go` | 中间件、前端内嵌、路由注册总入口 |
| `backend/internal/server/routes` | `auth`、`user`、`admin`、`gateway`、`payment` 路由注册 |
| `backend/internal/handler` | HTTP handler 和 gateway handler；`handler/dto` 放 DTO 映射 |
| `backend/internal/service` | 业务核心，包含 gateway、billing、scheduler、ops、payment、subscription、user 等 |
| `backend/internal/repository` | Ent、Redis、上游 HTTP、迁移 runner 和缓存 |
| `backend/ent/schema` | 数据模型源 |
| `backend/migrations` | SQL 迁移历史 |
| `backend/internal/web` | 嵌入式前端服务、HTML 缓存、public settings 注入 |

## 前端关键目录

| 路径 | 说明 |
| --- | --- |
| `frontend/src/router/index.ts` | 路由、鉴权、backend mode、simple mode、支付开关、chunk error 恢复 |
| `frontend/src/api/client.ts` | Axios client、token refresh、响应解包、语言/timezone 注入 |
| `frontend/src/api` | 用户侧 API 模块 |
| `frontend/src/api/admin` | 管理侧 API 模块 |
| `frontend/src/stores` | Pinia stores |
| `frontend/src/components/common` | 通用 UI 控件 |
| `frontend/src/components/admin` | 管理端领域组件 |
| `frontend/src/views/admin/ops` | 运维监控页面和组件 |
| `frontend/src/views/admin/orders` | 管理端支付订单/套餐页面 |
| `frontend/src/i18n/locales` | 中英文语言包 |

## 集成点

- `frontend/vite.config.ts` 将构建输出写到 `backend/internal/web/dist`。
- `backend/internal/web` 在 release/embed 构建中提供 SPA 静态资源和 public settings 注入。
- 前端 API 默认访问 `/api/v1`，Vite dev server 将 `/api`、`/v1`、`/setup` 代理到后端。
- 外部 AI 客户端直接访问后端 `/v1`、`/v1beta`、`/responses`、`/antigravity/*` 网关接口。
- Dockerfile 先构建前端，再复制前端 dist 到后端构建上下文，以 `-tags embed` 构建单二进制。

## 配置文件

| 路径 | 用途 |
| --- | --- |
| `backend/go.mod` | Go 版本和后端依赖 |
| `frontend/package.json` | 前端脚本和依赖范围 |
| `frontend/pnpm-lock.yaml` | 前端锁定版本 |
| `frontend/vite.config.ts` | dev/build 配置 |
| `frontend/vitest.config.ts` | 测试配置 |
| `frontend/tailwind.config.js` | Tailwind 配置 |
| `backend/.golangci.yml` | Go lint 规则 |
| `deploy/config.example.yaml` | 后端配置样例 |
| `deploy/.env.example` | Docker/环境变量样例 |
| `.github/workflows/backend-ci.yml` | CI |
| `.github/workflows/release.yml` | 发布 |
| `.github/workflows/security-scan.yml` | 安全扫描 |

## 文件组织模式

- 后端测试以 `*_test.go` 分布在相邻 package。
- 前端测试以 `*.spec.ts` 或 `*.test.ts` 分布在 `__tests__` 或相邻目录。
- 生成文件包括 `backend/ent/*`、`backend/cmd/server/wire_gen.go`，不要手改。
- 前端通用组件、API、store、utils 均按领域拆分。

## 资产位置

- README/赞助商图片：`assets/partners/logos`
- 前端公开 logo：`frontend/public/logo.png`
- 前端支付图标：`frontend/src/assets/icons`
- 后端模型价格资源：`backend/resources/model-pricing`

## 开发注意

- 根目录 Dockerfile 是生产多阶段构建入口；`backend/Dockerfile` 不是主要生产入口。
- 修改前端构建配置时确认产物仍输出到 `backend/internal/web/dist`。
- 涉及多语言 README 时注意 `README_CN.md` 当前疑似编码损坏，不要无意扩大乱码。
