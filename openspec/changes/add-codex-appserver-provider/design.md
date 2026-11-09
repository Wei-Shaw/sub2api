## Context

官方 `codex app-server` 使用双向 JSON-RPC 2.0 语义，并在 stdio 模式下以逐行 JSON 传输。客户端必须先调用 `initialize`，再发送 `initialized`，随后才能调用 `thread/start`、`turn/start` 和 `turn/interrupt`。响应与 notification 共享 stdout，因此需要一个请求 ID 路由器和一个事件流。

## Goals / Non-Goals

**Goals:**

- 真实覆盖子进程、stdin/stdout framing、request/response 路由和 notification。
- worker 生命周期与单次 turn context 分离；取消 turn 不直接杀 worker。
- 保持包边界小，使后续 service/provider 适配无需复制 JSON-RPC 实现。
- 测试不依赖真实 Codex、账号或网络。

**Non-Goals:**

- 不接入现有 `/v1/responses` 路由。
- 不实现 OAuth、`CODEX_HOME`、worker pool、sticky session、工具/审批桥或多账号故障转移。
- 不伪装 first-party `clientInfo`，不实现或绕过 attestation。
- mock E2E 不作为真实 OpenAI 上游验证。

## Decisions

### D1：独立包持有进程与协议

`internal/codexapp` 同时拥有命令启动、JSON-RPC client 和最小 provider。现有 OpenAI gateway 不感知 stdio 细节，后续仅通过窄接口接入。

### D2：单 worker、单活动 turn

首版 `Provider` 用互斥锁串行执行 turn。这样避免在尚未建立 thread/turn notification 多路复用前引入错误并发语义。worker pool 与 session affinity 留给后续独立变更。

### D3：取消发送 `turn/interrupt`

worker 由 provider 生命周期管理，不绑定单次 HTTP context。turn context 取消时使用短超时的独立 context 调用 `turn/interrupt`，随后返回原 context 错误。

### D4：fake app-server 是真实子进程

测试复用 Go test binary 作为 helper process，按场景读写 JSONL。它验证 OS pipe、framing、响应路由与通知顺序，而不是只替换 Go interface。

## Risks / Trade-offs

- [app-server 协议可能漂移] → 类型只覆盖稳定最小面；错误包含 method 与 RPC code；后续用真实 app-server + fake Responses upstream 做协议差分。
- [Scanner 默认 token 限制] → client 显式设置有界最大消息大小，超限快速失败。
- [stderr 阻塞子进程] → 始终并发 drain stderr，默认丢弃，允许调用方注入 writer。
- [mock 通过被误解为真实上游通过] → OpenSpec 与测试命名明确标注 hermetic/fake 边界。
