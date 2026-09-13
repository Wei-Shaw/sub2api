# 项目代码地图（CODEMAP）

> 目的：帮助人和 AI 快速定位代码，而不是重复解释代码做什么（那部分请直接看代码/测试）。
> 项目：`github.com/Wei-Shaw/sub2api`，Go 后端（Ent ORM + Gin）+ Vue3 前端（pnpm）。
> 其他文档：环境配置/CI/坑点见 [DEV_GUIDE.md](DEV_GUIDE.md)，测试约束见 [AGENTS.md](AGENTS.md)。

## 一、顶层目录

| 目录 | 内容 |
|------|------|
| `backend/` | Go 后端服务 |
| `frontend/` | Vue3 前端（pnpm 管理，非 npm） |
| `deploy/` | Docker Compose / Caddy / 安装脚本 / 发布相关文档 |
| `docs/` | 功能性设计文档（支付、异步图片任务、插件开发等） |
| `openspec/` | 变更提案（changes/）与规范配置 |
| `skills/`、`tools/` | 辅助脚本/技能 |

## 二、后端 `backend/`

### 入口与生成代码

| 路径 | 内容 |
|------|------|
| `cmd/server/main.go` | 服务主入口 |
| `cmd/server/wire.go` / `wire_gen.go` | 依赖注入（Google Wire），改了 provider 需要 `go generate` 或 `wire` 重新生成 |
| `cmd/jwtgen/`、`cmd/profit-preview/`、`cmd/cleanup-ingress-reject-logs/` | 独立小工具 |
| `ent/` | Ent ORM 生成代码；`ent/schema/*.go` 是 Schema 源，改完必须 `go generate ./ent` |
| `migrations/` | SQL 迁移脚本（308+ 个，按序号命名） |

### `internal/` 分层（核心业务代码都在这里）

| 目录 | 职责 | 典型定位关键词 |
|------|------|----------------|
| `handler/` | HTTP Handler，解析请求/组装响应，调用 service | `*_handler.go`；网关相关看 `gateway_handler*.go`、`openai_gateway_handler.go`、`gemini_v1beta_handler.go`；管理端在 `handler/admin/` |
| `handler/dto/` | 请求/响应 DTO 与字段映射（mapper） | `mappers.go`、`*_mapper_*` |
| `handler/quotaview/` | 配额展示辅助 | — |
| `service/` | 业务逻辑层，**文件数量最多（1200+）**，按前缀分组 | `account*` 账号池/调度、`usage*`/`usage_log*` 用量计费、`token_refresh*`/`token_cache*` Token 刷新与缓存、`upstream_*` 上游探测与限流、`payment` 相关另见 `internal/payment/`、`websearch_config.go` 联网搜索配置 |
| `repository/` | 数据访问层（对接 Ent/DB/Redis） | `account_repo*.go`、`api_key_repo*.go`、`api_key_cache*.go`（Redis 缓存+订阅） |
| `domain/` | 纯领域模型/常量，不含 IO | `model_allowlist.go`、`reasoning_effort.go`、`constants.go` |
| `model/` | 少量独立数据模型 | `error_passthrough_rule.go`、`tls_fingerprint_profile.go` |
| `middleware/` | 独立中间件（速率限制） | `rate_limiter.go` |
| `server/` | HTTP 服务器装配 | `http.go`、`router.go`；`server/routes/` 按业务拆分路由（`admin.go`/`auth.go`/`gateway.go`/`payment.go`/`user.go`）；`server/middleware/` 是接入链路中间件（鉴权 `jwt_auth.go`/`api_key_auth*.go`、CORS、限流、审计日志、安全头等） |
| `securityaudit/` | 提示词安全审计/内容审核子系统（自成一个较大子模块） | `prompt_service.go` 核心服务、`prompt_worker.go` 异步处理、`prompt_guard.go`/`prompt_qwen3guard.go` 审核策略、`prompt_repository.go`/`prompt_payload_store.go` 存储 |
| `payment/` | 支付领域逻辑（金额/费率/汇率/支付方式注册） | `provider/` 下按渠道：`alipay.go`、`wxpay.go`、`stripe.go`、`airwallex.go`、`easypay.go` |
| `config/` | 配置加载与校验 | `config.go`、`validate_dingtalk.go` |
| `setup/` | 首次安装/初始化向导（CLI + HTTP） | `cli.go`、`setup.go` |
| `platform/liveattestation/` | 平台侧活体/证明相关 | — |
| `pkg/` | 对接各上游 AI 平台的协议适配层 | `pkg/openai/`、`pkg/claude/`、`pkg/gemini/`、`pkg/geminicli/`、`pkg/xai/`、`pkg/antigravity/`、`pkg/anthropicfp/`（指纹）、`pkg/tlsfingerprint/`、`pkg/oauth/`、`pkg/redissession/` |
| `util/` | 通用小工具 | `logredact/`（日志脱敏）、`urlvalidator/` |
| `integration/` | 端到端集成测试 | `e2e_gateway_test.go`、`e2e_user_flow_test.go` |
| `web/` | 前端静态资源嵌入（embed） | `embed_on.go`/`embed_off.go`、`dist/` |
| `testutil/` | 测试辅助 | — |

### 定位速查（按"我要改什么功能"）

| 想改的功能 | 去看这里 |
| --- | --- |
| AI 网关转发/协议适配（Chat/Responses/Embeddings/Images） | `handler/gateway_handler*.go`、`handler/openai_*.go`、`handler/gemini_v1beta_handler.go`、`pkg/{openai,claude,gemini,xai,antigravity}/` |
| 账号池调度/限流/故障切换 | `service/account*.go`（尤其 `account_scheduling_*`、`account_pool_*`）、`handler/failover_loop.go` |
| 用量统计与计费 | `service/usage_*.go`、`handler/usage_handler.go`、`handler/admin/usage_handler.go` |
| Token 刷新/缓存 | `service/token_refresh*.go`、`service/token_cache*.go` |
| 支付（下单/回调/渠道） | `internal/payment/`、`handler/payment_handler.go`、`handler/payment_webhook_handler.go`、`handler/admin/payment_handler.go` |
| 提示词安全审计/内容审核 | `internal/securityaudit/` |
| 渠道健康监控 | `handler/admin/channel_monitor_handler.go`、`handler/channel_monitor_v2_handler.go` |
| 认证/OAuth 登录 | `handler/auth_*.go`（钉钉/邮箱/OIDC/微信/LinuxDo）、`server/middleware/jwt_auth.go`、`server/middleware/api_key_auth*.go` |
| 管理端路由/权限 | `server/routes/admin.go`、`server/middleware/admin_auth.go` |
| Ops/运维看板 | `handler/admin/ops_*.go` |

## 三、前端 `frontend/src/`

| 目录 | 职责 |
|------|------|
| `views/` | 页面级组件，按角色分：`admin/`（管理端）、`user/`（用户端）、`auth/`（登录注册/OAuth 回调）、`public/`、`setup/`（安装向导） |
| `components/` | 可复用组件，按业务分子目录：`account/`、`admin/`、`channels/`、`charts/`、`common/`、`icons/`、`keys/`、`layout/`、`modelPlaza/`、`payment/`、`user/`、`auth/` |
| `features/` | 按特性纵向切分的模块（自带 api/组件/类型），如 `prompt-audit/`、`prompt-records/`、`channel-monitor-v2/` |
| `api/` | 后端 API 调用封装，一文件对应一类后端资源（`accounts.ts`、`channels.ts`、`payment.ts`…），`admin/` 子目录对应管理端接口 |
| `stores/` | Pinia store（`auth.ts`、`app.ts`、`payment.ts`、`subscriptions.ts`…） |
| `composables/` | 可复用逻辑（`use*.ts`），如各平台 OAuth（`useOpenAIOAuth.ts` 等）、表格/表单通用逻辑 |
| `router/` | 路由定义与守卫（`index.ts`、`setupRedirect.ts`、`title.ts`） |
| `i18n/` | 国际化文案（`locales/`） |
| `styles/` | 全局样式（自定义主题、引导页、公告 Markdown 样式） |
| `types/` | 全局 TS 类型 |
| `utils/` | 通用工具函数（格式化、错误分类、设备识别等） |

### 定位速查（前端）

| 想改的功能 | 去看这里 |
| --- | --- |
| 管理端某个页面 | `views/admin/*View.vue`，复杂页面在 `views/admin/<模块>/` 下有子目录（如 `ops/`、`orders/`、`settings/`、`affiliates/`） |
| 用户端页面（订阅/支付/用量/密钥） | `views/user/*View.vue` |
| 登录/OAuth 回调 | `views/auth/`，对应逻辑在 `composables/use*OAuth.ts` |
| 提示词审计前端 | `features/prompt-audit/`、`features/prompt-records/` |
| 渠道监控前端（V2） | `features/channel-monitor-v2/` |
| 与后端某接口对接 | 先查 `api/` 下对应文件，再查 `api/admin/`（管理端专属接口） |

## 四、其他文档索引

| 位置 | 内容 |
|------|------|
| `DEV_GUIDE.md` | 本地环境配置、CI 要求、常见坑点 |
| `AGENTS.md` | 测试执行约束（Docker 优先、测试数据保留规则、本地登录账号） |
| `docs/*.md` | 具体功能设计文档（支付集成 API、异步图片任务、插件开发、ShowDoc 设计等） |
| `backend/dev-docs/*.md` | 后端专项说明（如上游错误代理归因） |
| `openspec/changes/` | 进行中的变更提案 |
