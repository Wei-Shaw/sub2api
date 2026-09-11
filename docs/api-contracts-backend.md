# Backend API 合约目录

**生成时间:** 2026-04-28  
**扫描级别:** Quick Scan  
**来源:** `backend/internal/server/routes/*`、`backend/internal/handler/endpoint.go`、`frontend/src/api/*`

## 响应约定

管理端和用户端业务 API 默认使用统一响应外壳：

```json
{
  "code": 0,
  "message": "ok",
  "data": {}
}
```

前端 `frontend/src/api/client.ts` 会在 `code === 0` 时返回解包后的 `data`，业务组件通常不应再假设响应仍带有 `{ code, data }` 外壳。网关兼容接口会按目标协议返回 Anthropic/OpenAI/Gemini 风格响应。

## 公共与设置接口

| 方法 | 路径 | 说明 | 鉴权 |
| --- | --- | --- | --- |
| GET | `/health` | 健康检查 | 无 |
| GET | `/setup/status` | setup 状态兼容接口 | 无 |
| POST | `/api/event_logging/batch` | Claude Code 遥测兼容空处理 | 无 |
| GET | `/api/v1/settings/public` | 前端公共站点配置 | 无 |

## 认证接口

基础认证路径均在 `/api/v1/auth` 下，关键写入口启用 Redis rate limit，Redis 故障时按配置 fail-close。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/api/v1/auth/register` | 邮箱注册 |
| POST | `/api/v1/auth/login` | 登录 |
| POST | `/api/v1/auth/login/2fa` | 登录二次验证 |
| POST | `/api/v1/auth/send-verify-code` | 发送验证码 |
| POST | `/api/v1/auth/refresh` | refresh token |
| POST | `/api/v1/auth/logout` | 登出/撤销 refresh token |
| POST | `/api/v1/auth/forgot-password` | 忘记密码 |
| POST | `/api/v1/auth/reset-password` | 重置密码 |
| POST | `/api/v1/auth/validate-promo-code` | 验证优惠码 |
| POST | `/api/v1/auth/validate-invitation-code` | 验证邀请码 |
| GET/POST | `/api/v1/auth/oauth/*` | LinuxDo、WeChat、OIDC 和 pending OAuth 流程 |
| GET | `/api/v1/auth/me` | 当前用户 |
| POST | `/api/v1/auth/revoke-all-sessions` | 撤销所有会话 |

## 用户接口

登录用户接口挂载 JWT 鉴权与 backend mode 用户守卫。

| 分组 | 路径前缀 | 主要能力 |
| --- | --- | --- |
| Profile | `/api/v1/user` | profile、改密码、账户绑定、通知邮箱、TOTP、aff 转移 |
| API Keys | `/api/v1/keys` | API key 列表、详情、创建、更新、删除 |
| Groups | `/api/v1/groups` | 可用分组、用户分组费率 |
| Channels | `/api/v1/channels` | 用户可用渠道 |
| Usage | `/api/v1/usage` | usage 列表、详情、统计、dashboard 图表与批量 key usage |
| Announcements | `/api/v1/announcements` | 用户公告列表、标记已读 |
| Redeem | `/api/v1/redeem` | 兑换卡密、兑换历史 |
| Subscriptions | `/api/v1/subscriptions` | 订阅列表、有效订阅、进度、摘要 |
| Channel Monitors | `/api/v1/channel-monitors` | 用户侧渠道状态和监控状态 |

## 管理员接口

管理员接口统一在 `/api/v1/admin` 下，挂载 admin auth。

| 分组 | 路径前缀 | 主要能力 |
| --- | --- | --- |
| Dashboard | `/dashboard` | snapshot、实时指标、趋势、模型/分组/API key/用户统计、聚合回填 |
| Users | `/users` | 用户 CRUD、余额、API keys、usage、订阅、属性、RPM、绑定身份 |
| Groups | `/groups` | 分组 CRUD、容量/usage、费率倍数、RPM override、排序 |
| Accounts | `/accounts` | 上游账号 CRUD、测试、刷新、CRS 同步、批量、模型、临时不可调度 |
| Announcements | `/announcements` | 公告 CRUD、阅读状态 |
| OAuth | `/openai`, `/gemini`, `/antigravity`, `/accounts/*auth*` | 各平台 OAuth 授权与 token 管理 |
| Proxies | `/proxies` | 代理 CRUD、测试、质量检查、导入导出 |
| Redeem/Promo | `/redeem-codes`, `/promo-codes` | 卡密、优惠码和使用记录 |
| Settings | `/settings` | 系统设置、SMTP、admin API key、overload、stream timeout、rectifier、beta policy、web search |
| Data/Backup | `/data-management`, `/backups` | 数据管理代理、S3、备份任务、恢复 |
| Ops | `/ops` | 实时并发、告警、运行时配置、错误日志、请求详情、系统日志、dashboard |
| System | `/system` | 版本、检查更新、执行更新、回滚、重启 |
| Subscriptions | `/subscriptions`, `/groups/:id/subscriptions`, `/users/:id/subscriptions` | 订阅分配、延期、重置、撤销、查询 |
| Usage | `/usage` | usage 查询、统计、用户/key 搜索、清理任务 |
| User Attributes | `/user-attributes` | 用户属性定义、批量读取、排序 |
| Error Passthrough | `/error-passthrough-rules` | 错误透传规则 CRUD |
| TLS Fingerprint | `/tls-fingerprint-profiles` | TLS 指纹配置 CRUD |
| API Key Admin | `/api-keys/:id` | 管理员更新 key 分组 |
| Scheduled Tests | `/scheduled-test-plans` | 定时测试计划 CRUD 和结果 |
| Channels | `/channels` | 渠道、定价、模型映射 |
| Channel Monitors | `/channel-monitors`, `/channel-monitor-templates` | 渠道监控与请求模板 |
| Affiliates | `/affiliates/users` | 邀请返利用户配置 |

## 支付接口

支付接口在 `/api/v1/payment`、`/api/v1/payment/public`、`/api/v1/payment/webhook` 和 `/api/v1/admin/payment`。

| 分组 | 方法/路径 | 说明 |
| --- | --- | --- |
| 用户支付配置 | `GET /config`, `/checkout-info`, `/plans`, `/channels`, `/limits` | 获取支付展示和限制 |
| 用户订单 | `POST /orders`, `GET /orders/my`, `GET /orders/:id`, `POST /orders/:id/cancel`, `POST /orders/:id/refund-request` | 创建、查询、取消、申请退款 |
| 公开恢复 | `POST /payment/public/orders/verify`, `POST /payment/public/orders/resolve` | 匿名兼容校验与 resume token 恢复 |
| Webhook | `/payment/webhook/easypay`, `/alipay`, `/wxpay`, `/stripe` | 支付平台回调 |
| 管理支付 | `/admin/payment/dashboard`, `/config`, `/orders`, `/plans`, `/providers` | 支付 dashboard、配置、订单、套餐、支付实例 |

## AI 网关接口

网关接口不走 `/api/v1` 前缀，使用 API key 鉴权和分组/订阅校验。

| 方法 | 路径 | 协议/用途 |
| --- | --- | --- |
| POST | `/v1/messages` | Anthropic Messages；OpenAI 分组时自动转 OpenAI gateway |
| POST | `/v1/messages/count_tokens` | Anthropic count tokens；OpenAI 分组返回不支持 |
| GET | `/v1/models` | 模型列表 |
| GET | `/v1/usage` | 网关 usage |
| POST/GET | `/v1/responses`, `/v1/responses/*subpath` | OpenAI Responses 兼容；GET 用于 WebSocket |
| POST | `/v1/chat/completions` | OpenAI Chat Completions 兼容 |
| POST | `/v1/images/generations`, `/v1/images/edits` | OpenAI Images 兼容 |
| GET/POST | `/v1beta/models*` | Gemini 原生兼容 |
| POST/GET | `/responses`, `/backend-api/codex/responses` | Codex/Responses 直连别名 |
| GET/POST | `/antigravity/v1/*`, `/antigravity/v1beta/*` | Antigravity 专用平台路由 |

## 前端 API 模块映射

前端 API 模块与后端领域基本一一对应：

- 用户侧：`auth.ts`、`user.ts`、`keys.ts`、`usage.ts`、`payment.ts`、`subscriptions.ts`、`redeem.ts`、`channels.ts`、`channelMonitor.ts`
- 管理侧：`admin/accounts.ts`、`admin/users.ts`、`admin/groups.ts`、`admin/settings.ts`、`admin/ops.ts`、`admin/payment.ts`、`admin/channels.ts`、`admin/channelMonitor.ts` 等

## 后续深挖建议

Quick Scan 未抽取每个 handler 的请求/响应 DTO。需要精确 OpenAPI/字段级合约时，应对 `backend/internal/handler/dto`、`backend/internal/handler/*_handler.go`、`frontend/src/types` 和 `frontend/src/api` 做 deep/exhaustive scan。
