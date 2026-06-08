# Change: Add Codex High-Fidelity Relay Mode

## Why

当前 Codex WS 中转把线程身份、turn 级 sticky state 和上游连接复用混在了一起。这个设计既会把失活 continuation 连接放进首包关键路径，拉高 TTFT，也会在 `function_call_output` 等场景中通过“删 `previous_response_id` + 重放 input”破坏官方语义。

## What Changes

- 为 Codex 官方客户端新增一条高保真中转路径，仅对官方 Codex 家族请求生效
- 停止把 `prompt_cache_key` 当作 Codex 线程身份，也不再把它兜底提升为上游 `session_id`
- 停止把 `x-codex-turn-state` 和上游 WS 连接跨 turn 复用到下一轮请求
- 对 Codex 官方客户端关闭 `previous_response_not_found` 的自动改写恢复，保留原始请求语义
- 保持非 Codex 路径的既有兼容逻辑不变

## Impact

- Affected specs: `codex-relay`
- Affected code: OpenAI Responses handler、Codex WS forwarder、会话哈希生成、WS 回归测试
