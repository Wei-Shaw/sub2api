## ADDED Requirements

### Requirement: Codex HTTP Ingress May Use WSv2 Passthrough Upstream

当请求来自 Codex 官方客户端、通过 HTTP `/responses` 入站、账号显式启用了 `openai_passthrough` 且其 OpenAI WSv2 ingress mode 为 `passthrough` 时，系统 SHALL 保留 WSv2 上游决策，并通过现有 WSv2 转发器发送请求。

#### Scenario: Official Codex HTTP request keeps WSv2 upstream

- **GIVEN** 一个 Codex 官方客户端通过 HTTP 调用 `/responses`
- **AND** 命中的 OpenAI 账号启用了 `openai_passthrough`
- **AND** 该账号的 OpenAI WSv2 ingress mode 为 `passthrough`
- **WHEN** 网关完成上游路由决策
- **THEN** 请求应进入 WSv2 上游转发
- **AND** 结果应记录为 `ws_v2`

### Requirement: Non-Eligible HTTP Requests Keep Existing HTTP Behavior

不满足上述条件的 HTTP `/responses` 请求 SHALL 继续保持当前 HTTP 上游行为。

#### Scenario: Non-Codex HTTP request remains HTTP

- **GIVEN** 一个非 Codex 官方客户端通过 HTTP 调用 `/responses`
- **WHEN** 网关完成上游路由决策
- **THEN** 请求应继续使用 HTTP 上游

#### Scenario: Codex HTTP request without passthrough mode remains HTTP

- **GIVEN** 一个 Codex 官方客户端通过 HTTP 调用 `/responses`
- **AND** 命中的账号未启用 `openai_passthrough` 或其 OpenAI WSv2 ingress mode 不是 `passthrough`
- **WHEN** 网关完成上游路由决策
- **THEN** 请求应继续使用 HTTP 上游
