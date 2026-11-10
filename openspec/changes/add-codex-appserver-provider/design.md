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

### D5：官方 binary 黑盒测试在断网容器运行

可选 build-tag 测试使用固定版本官方 `@openai/codex`，通过 custom model provider 把 `base_url` 指向同一容器内的 `httptest.Server`。Docker 构建阶段允许下载固定镜像和 npm 包；测试运行阶段 MUST 使用 `--network none`、只读根文件系统、drop all capabilities、非 root 用户和临时文件系统。容器不挂载宿主目录或凭据。

黑盒测试从 `Provider.RunTurn` 进入，真实经过官方 app-server stdio JSON-RPC，再由官方 binary 发出 HTTP Responses 请求。fake upstream MUST 观察到假 Bearer token、目标模型、`stream:true` 与非空 input，并返回官方测试同形的 SSE 事件。测试完成后 MUST 验证临时 `CODEX_HOME` 未生成 `auth.json`。

## Risks / Trade-offs

- [app-server 协议可能漂移] → 类型只覆盖稳定最小面；错误包含 method 与 RPC code；后续用真实 app-server + fake Responses upstream 做协议差分。
- [Scanner 默认 token 限制] → client 显式设置有界最大消息大小，超限快速失败。
- [stderr 阻塞子进程] → 始终并发 drain stderr，默认丢弃，允许调用方注入 writer。
- [mock 通过被误解为真实上游通过] → OpenSpec 与测试命名明确标注 hermetic/fake 边界。
- [容器构建时需要公网下载依赖] → build 与 run 分离；只有 build 联网，最终 test run 强制 `--network none`，且镜像/官方 Codex 版本固定。
- [官方 app-server 测试被误解为完整 sub2api HTTP E2E] → 明确入口是 `codexapp.Provider`，尚未覆盖生产 `/v1/responses` handler、账号路由和计费。
