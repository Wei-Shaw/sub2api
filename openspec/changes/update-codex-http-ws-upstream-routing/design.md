## Context

当前 `Forward` 里的职责边界有一个明显耦合点：客户端入站协议被直接拿来决定上游协议，进而压过了账号级传输配置。对于普通兼容客户端，这种保守策略能降低不确定性；但对 Codex 官方客户端和已配置 `passthrough` 的 OpenAI OAuth 账号，它反而阻断了项目已经实现好的低延迟路径。

另一个耦合点是 `openai_passthrough` 既承载“请求语义尽量不改写”的含义，又承载“必须走 HTTP 上游”的隐式含义。后者并不是业务必需，只是当前分支顺序造成的副作用。

## Goals / Non-Goals

- Goals:
  - 让 Codex 官方客户端的 HTTP `/responses` 请求可以命中已配置的 WSv2 passthrough 上游
  - 保留 passthrough 账号现有的 body 规范化与本地拒绝规则
  - 不改变普通 HTTP 客户端和非 passthrough 账号的现有行为
- Non-Goals:
  - 不开放通用 HTTP 客户端到 WS 上游
  - 不让 `ctx_pool` 自动接管 HTTP 入站
  - 不重写 `forwardOpenAIWSV2` 或 WS 连接池状态机

## Decisions

- Decision: 仅允许 `HTTP + Codex official client + account passthrough + ws_v2 passthrough mode` 组合走 WSv2
  - Why: 这是已经被生产配置证明需要的最小集合，能避免把行为变化扩散到普通 OpenAI 兼容流量
  - Alternative considered: 所有 HTTP 入站都可走 WSv2
  - Rejected because: 行为面过大，会把现有 HTTP 兼容假设全部打散

- Decision: 继续复用现有 `forwardOpenAIWSV2`
  - Why: 转发、重试、计费、usage 解析和结果落库链路已经齐备，没必要再造一套 HTTP->WS 中转器
  - Alternative considered: 新建独立的 HTTP passthrough WS 转发器
  - Rejected because: 会复制状态处理和回归面，违背 KISS/DRY

- Decision: 为该路径新增轻量 passthrough body 预处理，而不是复用普通兼容改写链
  - Why: passthrough 账号的核心诉求是尽量保持官方语义，不能把 HTTP passthrough 账号悄悄变成普通兼容账号
  - Alternative considered: 直接绕过 passthrough 分支，让现有普通 WS 路径处理
  - Rejected because: 会引入额外字段改写、模型改写和兼容清洗，破坏“透传”边界

## Risks / Trade-offs

- 风险: HTTP 入站允许使用 WSv2 后，问题定位维度会从“入站协议”变成“入站协议 + 上游协议”
  - Mitigation: 给 transport reason 增加显式标记，并保持 request_type / OpenAIWSMode 自动落为 `ws_v2`

- 风险: passthrough 预处理和普通兼容预处理分成两条链，后续维护需要注意边界
  - Mitigation: 只抽出确实共享的 passthrough 预处理，不把两条链再揉回一个大函数

## Migration Plan

1. 为 HTTP 入站增加“可保留 WSv2 决策”的受限判定
2. 抽出 passthrough body 预处理，供 HTTP passthrough 与 HTTP->WS passthrough 共用
3. 调整分流顺序，让受限场景先进入 WSv2
4. 补齐回归测试并验证本地构建
