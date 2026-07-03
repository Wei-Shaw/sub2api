# Change: Update Codex HTTP Ingress WS Upstream Routing

## Why

当前项目已经具备 HTTP `/responses` -> OpenAI WSv2 的转发能力，但生产主路径仍被两层旧策略提前拦回 HTTP：
- `client_protocol_http` 会把所有 HTTP 入站统一降级为 HTTP 上游
- `openai_passthrough` 会在 WS 路由决策之前直接短路到 HTTP 透传

这导致账号明明配置了 `responses_websockets_v2_mode=passthrough`，Codex 官方客户端的主流量仍然无法进入 WSv2，TTFT 改善空间被硬性封死。

## What Changes

- 仅对 Codex 官方客户端的 HTTP `/responses` 请求，允许保留 WSv2 上游决策
- 仅当账号已显式启用 `openai_passthrough=true` 且 WSv2 ingress mode 为 `passthrough` 时，放开上述保留
- 让该路径复用现有 WSv2 转发器，但继续沿用 passthrough 请求体规范化语义
- 保持非 Codex 客户端、非 passthrough 账号、WS disabled 账号的现有 HTTP 行为不变
- 补充 HTTP 入站走 WSv2 与 HTTP 入站保持 HTTP 的回归测试

## Impact

- Affected specs: `codex-relay`
- Affected code: OpenAI transport 分流、OpenAI passthrough 预处理、WSv2 回归测试
