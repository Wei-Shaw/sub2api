## Context
`sub2api` 当前的 OpenAI gateway 已能处理 `/v1/responses`、`/v1/chat/completions` 到 Responses 的兼容转换、账号调度、用量记录和自定义 OpenAI APIKey `base_url`。这可覆盖 xAI 官方 API Key 的部分文本兼容场景，但不能覆盖 Grok Build OAuth 账号，也不能保证 xAI 对 Responses 字段、tools 和 reasoning 的约束。

CLIProxyAPI 的 Grok 实现分为两层：
- `xai` OAuth：OIDC discovery、PKCE、loopback callback、token exchange/refresh、凭证保存。
- `xai` executor：把入站请求转为 xAI Responses 请求，发送到 `/responses`，并对 body 做 xAI 专用清洗。

Sub2API 应吸收协议层经验，但保持本项目的 `handler -> service -> account scheduler -> usage/billing` 分层。

## Goals / Non-Goals
- Goals:
  - 支持 Grok Build OAuth 文本账号作为 `xai` provider。
  - 支持 `/v1/responses` 和 `/v1/chat/completions` 入站调用 Grok 文本模型。
  - 成功请求继续进入现有调度、并发、计费、用量日志链路。
  - xAI token 过期前可自动 refresh。
  - xAI 上游请求不携带已知会被拒绝或语义不匹配的字段。
- Non-Goals:
  - 不在本次实现 Grok 图片或视频接口。
  - 不引入 CLIProxyAPI 的 executor/auth manager 框架。
  - 不改变现有 OpenAI、Gemini、Claude、Antigravity 行为。
  - 不把 xAI API Key 伪装成 OpenAI 平台；`xai` 应作为独立平台展示和统计。

## Decisions
- Decision: 使用独立 `PlatformXAI = "xai"`。
  - Why: 账号、模型、用量、错误策略和 OAuth 生命周期与 OpenAI 不同，独立平台能避免配置和统计混淆。
- Decision: 第一阶段仅实现文本 Responses API。
  - Why: 图片/视频涉及新路由、请求/响应转换和 image/video 计费，应拆分降低风险。
- Decision: 复用现有 OpenAI 入站兼容转换，但在上游发送前进入 xAI 专用清洗函数。
  - Why: 入站协议与 OpenAI Responses/Chat Completions 接近，复用能减少重复；xAI 上游差异必须显式隔离。
- Decision: xAI OAuth 凭证存在 `accounts.credentials`，包括 `access_token`、`refresh_token`、`expired`、`base_url`、`token_endpoint`、`email/sub`。
  - Why: 与现有 OAuth 账号持久化模式一致，并为 refresh 和后台展示保留必要字段。
- Decision: xAI 云端模型通过后台账号级手动刷新写入 `accounts.extra.cloud_models` 快照，网关 `/v1/models` 只读取快照。
  - Why: 学习 CLIProxyAPI 的远端目录/缓存思路，同时避免模型列表请求在调度热路径触发上游网络调用；手工 `model_mapping` 仍然优先，保证现有限制/改写语义不变。

## Upstream Protocol Notes
- OAuth discovery endpoint: `https://auth.x.ai/.well-known/openid-configuration`
- Public OAuth client ID: `b1a00492-073a-47ea-816f-4c329264a828`
- Scope: `openid profile email offline_access grok-cli:access api:access`
- Default callback: `http://127.0.0.1:56121/callback`
- Default API base URL: `https://api.x.ai/v1`
- Text endpoint: `POST /responses`
- Required auth header: `Authorization: Bearer <access_token>`
- Streaming accept header: `Accept: text/event-stream`
- JSON accept header: `Accept: application/json`
- Optional conversation header: `x-grok-conv-id`

## Request Normalization
Before forwarding to xAI text endpoint, the system should:
- set `model` to the mapped Grok model and `stream` to the effective stream mode;
- delete `previous_response_id`, `prompt_cache_retention`, `safety_identifier`, and `stream_options`;
- use `prompt_cache_key` as `x-grok-conv-id`/conversation hint when available, without treating it as Sub2API session ownership;
- remove `reasoning` for models that do not support Grok reasoning effort;
- remove unsupported tool types such as `tool_search` and `image_generation`;
- convert custom tools to function tools when possible;
- remove `tool_choice` and `parallel_tool_calls` when no tools remain;
- drop `include: ["reasoning.encrypted_content"]` because xAI does not return OpenAI encrypted reasoning in the same form.

## Compatibility Contract
This change can make compatibility testable, but it cannot promise byte-for-byte parity with OpenAI or future xAI behavior.

- Existing provider compatibility: OpenAI, Gemini, Claude, and Antigravity request routing, account selection, model mappings, billing, and usage logs must remain covered by existing tests plus targeted no-regression checks.
- Inbound client compatibility: `/v1/responses`, `/v1/chat/completions`, and `/v1/models` should keep Sub2API's existing response shapes for clients while selecting `xai` accounts through group routing.
- xAI upstream compatibility: requests must be normalized before `POST /responses` so xAI does not receive fields and tool shapes known to be unsupported by Grok.
- Streaming compatibility: xAI SSE events should be relayed in Responses-compatible form, including patching a final completed event from collected `response.output_item.done` events when xAI omits `response.output`.
- Semantic limits: unsupported tool types, encrypted reasoning content, image generation, and video generation are intentionally not preserved in phase one; clients that require those behaviors are not fully compatible until a later media/tool proposal covers them.

## Risks / Trade-offs
- OAuth endpoints or scopes may change. Mitigation: use discovery for endpoints and keep OAuth constants isolated.
- xAI Responses SSE may omit `response.output` in the final completed event. Mitigation: collect `response.output_item.done` events and patch completed output if needed.
- Existing OpenAI gateway code is large. Mitigation: prefer small xAI-specific helpers or service methods over broad refactors.
- Model list can drift. Mitigation: keep a static default list, allow admin-configured model mappings/whitelists, and support manual cloud model refresh snapshots per xAI account.

## Migration Plan
1. Add platform constants and account support without changing existing platform behavior.
2. Add xAI OAuth service and admin/OAuth endpoints.
3. Add text forwarding path with xAI request normalization.
4. Add backend unit tests for OAuth URL construction, token refresh metadata, body normalization, and forwarding URL/header construction.
5. Add frontend platform/OAuth/model form support.
6. Run targeted Go and frontend tests.

## Open Questions
- 是否需要本次同时支持 xAI 官方 API Key 作为 `xai` 平台账号，还是只支持 Grok Build OAuth？
- 后台 OAuth callback 是否沿用现有 OpenAI/Gemini OAuth UI，还是新增独立 Grok 登录入口？
