# Add Devin (Cognition) Platform

## Why

Sub2API 当前支持 Anthropic / OpenAI / Gemini / Antigravity / Grok / 国产供应商等平台。Devin（Cognition 的 AI 工程师产品）通过 Connect RPC 暴露会话式聊天能力（`server.codeium.com` 的 `DevinService/GetChatMessage` 等），协议是 Connect 帧 + 自定义 proto wire 格式，与任何现有平台都不兼容。本变更把 Devin 账号（PKCE 登录 / session token 直贴）纳入统一账号池，并把入站 Responses / Chat Completions / Anthropic Messages 三类标准 API 翻译为 Devin Connect RPC，实现 Devin 账号的分发与反代。

## What Changes

- 新增 `internal/pkg/devin` wire 层：手工实现的 proto 编解码（字段号以本地 devin-connect 插件抓包为准）、Connect 帧编解码、CLI 元数据头、PKCE 授权码交换（`ExchangePKCEAuthorizationCode` → `ExchangeDevinCLIPKCECode` 兜底）、目录解码（`ListModels` → `GroupModels` 按族聚合 + thinkingLevelMap）、限流错误分类。
- 新增 `internal/pkg/devin/llm` 协议无关 IR 与 `internal/pkg/devin/adapter`：会话派生（sha256 → 确定性 UUID）、请求编码（step_index 0 起步，跳过 1 让位标题生成）、工具定义转换、响应解码（trajectory/step_index 归属）、sanitize、流式泵。
- 新增入站 codec（`internal/pkg/devin/api/...`）：Anthropic `/v1/messages`、OpenAI `/v1/chat/completions`、OpenAI `/v1/responses`（含 GET WebSocket 传输，会话状态机 `WSSession` 做增量 transcript 归并 / 孤儿输出配对 / pending tool call 门控 / replacement replay）。
- 新增 `PlatformDevin` 平台：`/admin/devin/oauth/{auth-url,exchange-code}` 管理端点（exchange 对 `devin-session-token$…` 直贴免会话）、`DevinGatewayHandler`（messages/chat_completions/responses + GET WS + /v1/models 目录）、`DevinGatewayService`（per-account adapter、Stream、ListModels、`DevinFailoverError` 分类：本地请求形状错误 → 400 停止，传输层错误 → 502 failover）、`DevinQuotaFetcher`（GetUserStatus → 额度快照并入 UsageInfo.devin_quota）、账号连通性测试。
- 调度复用 `SelectAccountWithLoadAwareness`；账号级 `model_mapping` 在选中账号后改写上游模型（客户端模型仍用于计费/响应回填）。复合分组路由识别 `devin`/`cognition` 平台别名与 `swe-*`/`devin-*` 模型前缀。
- 前端：平台选项/图标/徽标/配色（sky）、`useDevinOAuth` composable、CreateAccountModal 平台按钮 + OAuth 授权流程、ReAuthAccountModal 换绑、AccountUsageCell 额度窗口（24h/7d 剩余百分比条 + credits/ACU 行）、`devinModels` 白名单目录、中英 i18n。

## Capabilities

### New Capabilities
- `devin-platform`：Devin 账号凭据模型、PKCE 登录、token 直贴、额度探测、连通性测试。
- `devin-gateway`：三类入站协议 → Devin Connect RPC 的翻译转发、流式泵、WS 传输、模型目录、failover 分类与计费接入。

### Modified Capabilities
- `group-routing`：`platform`/`target_platform` oneof 与复合平台检测新增 `devin`；分组默认模型候选新增 Devin 目录。

## Impact

- **数据库**：无 schema 变更；`accounts.platform='devin'`、`credentials.access_token`/`api_server_url`/`client_version`，`extra.email`/`org_id`/`plan_name`/`devin_quota`。
- **后端**：新增 `internal/pkg/devin{,/llm,/adapter,/api/...}` 与 `internal/service/devin_*.go`、`internal/handler/devin_gateway_*.go`；改动 `routes/gateway.go`（POST/GET `/responses`、POST `/chat/completions` 根别名 + `/api/v1` 分派）、`routes/admin.go`、`handler/endpoint.go`、`service/{account,account_usage_service,account_test_service,admin_group,composite_platform,wire}.go`、`cmd/server/wire.go`（+wire_gen.go 重生成）。
- **前端**：`api/admin/devin.ts`、`composables/useDevinOAuth.ts`、`useModelWhitelist`（devinModels）、`utils/platformColors`（sky 色系）、`PlatformIcon`/`PlatformTypeBadge`/`UseKeyModal`、`CreateAccountModal`/`ReAuthAccountModal`/`OAuthAuthorizationFlow`/`AccountUsageCell`、`types/index.ts`、`constants/platforms.ts`、en/zh i18n。
- **配置**：Devin WS 复用 `Gateway.OpenAIWS` 的 ingress/首包/读限参数，无新增配置项。
- **风险**：wire 字段号基于 v3000.10.21 抓包（上游变号需回归）；`client_version` 可能被上游门控（可在账号 credentials 覆盖）；ACU/credit 计费依赖 rate_multiplier。
