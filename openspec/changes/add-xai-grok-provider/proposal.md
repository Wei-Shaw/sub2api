# Change: 集成 xAI/Grok 文本 Provider

## Why
Sub2API 当前可以通过 OpenAI APIKey + 自定义 `base_url` 临时转发部分 xAI 兼容接口，但缺少 Grok Build OAuth 登录、token refresh、xAI 专用请求清洗和模型/账号调度的一等 provider 支持。

上游 CLIProxyAPI 已把 Grok 作为 `xai` provider 实现：OAuth 凭证登录后转发到 xAI Responses API，并在请求进入上游前处理 xAI 不支持的字段和工具形态。Sub2API 需要以本项目现有账号、分组、计费、调度边界集成，而不是直接复用外部项目的运行框架。

## What Changes
- 新增 `xai` 平台常量、账号类型支持和后台创建/测试路径。
- 新增 xAI OAuth 登录、callback、token exchange、refresh 和凭证持久化能力。
- 新增 Grok 文本模型的 OpenAI Responses/Chat Completions 入站兼容转发。
- 新增 xAI 上游请求清洗：删除 xAI 不支持的 Responses 字段，规范工具定义和 reasoning 参数。
- 新增 Grok 模型列表展示、默认模型集合和后台账号级云端模型刷新快照，覆盖 `grok-4.3`、`grok-3-mini` 等文本模型。
- 保持图片/视频接口不在本次范围内，后续单独 proposal。

## Impact
- Affected specs: `xai-grok-provider`
- Affected code:
  - `backend/internal/domain/constants.go`
  - `backend/internal/service/account*.go`
  - `backend/internal/service/oauth*.go` 或新增 `xai_oauth_service.go`
  - `backend/internal/service/openai_gateway_service.go` 或新增 `xai_gateway_service.go`
  - `backend/internal/handler/*openai*` / admin account handlers
  - `backend/internal/server/router.go`
  - `frontend/src/**` 后台账号、OAuth、模型选择相关页面
- External dependencies:
  - xAI OAuth discovery: `https://auth.x.ai/.well-known/openid-configuration`
  - xAI API base URL: `https://api.x.ai/v1`
