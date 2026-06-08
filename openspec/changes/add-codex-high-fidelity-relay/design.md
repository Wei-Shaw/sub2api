## Context

Sub2API 当前的 Codex WS 路径为了降低连接开销，引入了 `sessionHash -> turn_state`、`sessionHash -> conn`、`response_id -> conn` 等多层缓存，并在恢复时对请求体进行自动改写。这在通用兼容模式下有一定容错价值，但对 Codex 官方客户端来说，已经越过了“中转”边界，导致状态语义和连接生命周期发生耦合。

## Goals / Non-Goals

- Goals:
  - 让 Codex 官方客户端的 thread identity、turn state、transport lifecycle 各自归位
  - 切断跨 turn 失活连接对 TTFT 的阻塞
  - 保留 `previous_response_id` 和 `function_call_output` 的原始语义
  - 不影响非 Codex 路径的现有兼容行为
- Non-Goals:
  - 这次不实现完整的 tool-call graph registry
  - 这次不重写整个 OpenAI 调度器或连接池
  - 这次不新增面向用户的配置开关

## Decisions

- Decision: 用现有官方客户端识别逻辑作为高保真模式开关
  - Why: 需求已经被 Codex 官方客户端验证存在；不需要再引入额外配置和分流体系
  - Alternative considered: 新增全局配置开关
  - Rejected because: 会增加运维分歧和测试矩阵，且不能根除职责混乱

- Decision: Codex 高保真模式下，线程身份只认 `session_id` / `conversation_id`
  - Why: `prompt_cache_key` 是缓存提示，不是官方会话主键
  - Alternative considered: 继续将 `prompt_cache_key` 兜底提升为 `session_id`
  - Rejected because: 会把缓存提示错误提升为线程身份，继续制造隐式耦合

- Decision: Codex 高保真模式下，`x-codex-turn-state` 只在单 turn 内保存
  - Why: 官方客户端把它定义为 turn-scoped sticky token，不应跨 turn 发送
  - Alternative considered: 继续沿用 `sessionHash -> turn_state`
  - Rejected because: 这正是当前 TTFT 长尾和 contract 偏离的根因之一

- Decision: Codex 高保真模式下，每个 turn 都强制新建上游 WS 连接
  - Why: 旧连接是否仍然存活不应成为新 turn 的前置条件
  - Alternative considered: 继续走连接池复用，但只关闭 strict affinity
  - Rejected because: 连接级残留上下文仍可能泄漏到下一 turn，问题边界不够清晰

- Decision: Codex 高保真模式下，禁用 `previous_response_not_found` 的自动删锚点恢复
  - Why: 自动改写会把原始请求语义变成“网关猜测语义”，尤其会破坏工具调用链
  - Alternative considered: 保留现有恢复，但对 `function_call_output` 特判
  - Rejected because: 这仍然把传输恢复和会话语义恢复混在一起

## Risks / Trade-offs

- 风险: 每 turn 新建 WS 连接会增加握手次数
  - Mitigation: 仅对官方 Codex 客户端启用；机器基线握手成本远小于当前长尾 TTFT

- 风险: 极少数依赖 `prompt_cache_key` 兜底会话的旧客户端可能失去粘连
  - Mitigation: 高保真模式只覆盖官方 Codex 客户端，其他兼容客户端保持现状

## Migration Plan

1. 新增 Codex 高保真判定与会话哈希策略
2. 收紧 WS forwarder 的 turn-state / connection reuse 逻辑
3. 调整恢复策略，只保留语义安全的重试
4. 用现有回归测试覆盖 HTTP WSv2 与 ingress WS 两条主路径

## Open Questions

- 后续若要进一步逼近官方客户端实现，是否要补 `items_added` 级别的 turn baseline，而不只依赖请求 `input`
