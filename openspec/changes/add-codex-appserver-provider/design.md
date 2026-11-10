## Context

官方 `codex app-server` 使用双向 JSON-RPC 2.0 语义，并在 stdio 模式下以逐行 JSON 传输。客户端必须先调用 `initialize`，再发送 `initialized`，随后才能调用 `thread/start`、`turn/start` 和 `turn/interrupt`。响应与 notification 共享 stdout，因此需要一个请求 ID 路由器和一个事件流。

## Goals / Non-Goals

**Goals:**

- 真实覆盖子进程、stdin/stdout framing、request/response 路由和 notification。
- worker 生命周期与单次 turn context 分离；取消 turn 不直接杀 worker。
- 同一账号复用常驻 worker，同一用户会话复用 thread，避免每请求启动进程和丢失上下文。
- 从用户 HTTP 输入到 Responses 输出覆盖实际 handler、账号选择与计费记录入口。
- 保持包边界小，使后续 service/provider 适配无需复制 JSON-RPC 实现。
- 测试不依赖真实 Codex、账号或网络。

**Non-Goals:**

- 不实现 OAuth/`CODEX_HOME` 物化、工具/审批桥或多账号故障转移。
- 不在本变更中自动创建生产 pool；部署侧账号凭据隔离与 worker factory 仍需后续显式接线。
- 不伪装 first-party `clientInfo`，不实现或绕过 attestation。
- mock E2E 不作为真实 OpenAI 上游验证。

## Decisions

### D1：独立包持有进程与协议

`internal/codexapp` 同时拥有命令启动、JSON-RPC client 和最小 provider。现有 OpenAI gateway 不感知 stdio 细节，后续仅通过窄接口接入。

### D2：单 worker、单活动 turn

`Provider` 用互斥锁串行执行 turn。pool 按账号 key 复用一个常驻 provider，并按 session key 保存 thread ID；同一账号的并发 turn 排队，不同账号可由不同进程并行。pool 达到容量且所有 worker 忙时快速返回 exhausted，不在请求路径无界创建进程。

账号选择、负载评分、并发 slot、等待队列和 failover 继续由现有 `OpenAIAccountScheduler` 负责；worker pool 不读取账号负载，也不决定换号。由于当前每账号只有一个串行 worker，app-server 账号在 scheduler 中的实际并发与 load factor 固定为 1，避免把请求隐藏排在 worker mutex 后。pool 达到 `MaxWorkers` 时先淘汰最久未使用的 idle worker；只有全是 active worker 才返回 exhausted。

### D3：取消发送 `turn/interrupt`

worker 由 provider 生命周期管理，不绑定单次 HTTP context。turn context 取消时使用短超时的独立 context 调用 `turn/interrupt`，随后返回原 context 错误。

### D4：fake app-server 是真实子进程

测试复用 Go test binary 作为 helper process，按场景读写 JSONL。它验证 OS pipe、framing、响应路由与通知顺序，而不是只替换 Go interface。

### D5：官方 binary 黑盒测试在断网容器运行

可选 build-tag 测试使用固定版本官方 `@openai/codex`，通过 custom model provider 把 `base_url` 指向同一容器内的 `httptest.Server`。Docker 构建阶段允许下载固定镜像和 npm 包；测试运行阶段 MUST 使用 `--network none`、只读根文件系统、drop all capabilities、非 root 用户和临时文件系统。容器不挂载宿主目录或凭据。

黑盒测试从 `Provider.RunTurn` 进入，真实经过官方 app-server stdio JSON-RPC，再由官方 binary 发出 HTTP Responses 请求。fake upstream MUST 观察到假 Bearer token、目标模型、`stream:true` 与非空 input，并返回官方测试同形的 SSE 事件。测试完成后 MUST 验证临时 `CODEX_HOME` 未生成 `auth.json`。

### D6：用户 HTTP 适配复用现有 handler

账号必须显式设置 `openai_execution_backend=codex_app_server`，service 才把 Responses 文本输入交给 pool；其他账号继续走原 direct forwarding。适配层将 app-server delta 还原为 Responses JSON/SSE，并把 `thread/tokenUsage/updated` 的 `last` 用量写入既有 `OpenAIForwardResult`，使现有 usage recording 路径得到本轮 token，而不是零计费。

用户黑盒测试通过真实 HTTP socket 调用 `/openai/v1/responses` 两次，使用测试鉴权上下文和测试账号仓库，断言只创建一个 app-server 进程，第二次请求把第一次返回的 thread ID 交回同一 worker。官方 binary 的上游仍是容器 loopback fake Responses；测试不接触真实账号。

## Risks / Trade-offs

- [app-server 协议可能漂移] → 类型只覆盖稳定最小面；错误包含 method 与 RPC code；后续用真实 app-server + fake Responses upstream 做协议差分。
- [Scanner 默认 token 限制] → client 显式设置有界最大消息大小，超限快速失败。
- [stderr 阻塞子进程] → 始终并发 drain stderr，默认丢弃，允许调用方注入 writer。
- [mock 通过被误解为真实上游通过] → OpenSpec 与测试命名明确标注 hermetic/fake 边界。
- [容器构建时需要公网下载依赖] → build 与 run 分离；只有 build 联网，最终 test run 强制 `--network none`，且镜像/官方 Codex 版本固定。
- [HTTP 黑盒测试被误解为真实部署验收] → 测试覆盖 handler、账号选择、usage recording 入口、pool 与官方 binary，但鉴权/账号仓库是测试夹具，上游是假 Responses，且生产 worker factory 尚未接线。
- [worker 崩溃导致内存 thread 粘性丢失] → 不健康 worker 会被丢弃并在下次请求重建；本变更不声称跨进程恢复会话，持久化 resume 留作后续能力。
- [跨账号 failover 静默丢失 thread 上下文] → 当前 adapter 不把 app-server worker 错误包装为可换号错误；已有会话依赖现有 sticky/previous-response 绑定留在原账号。没有可验证 resume/replay 前，不声明支持有状态会话跨账号迁移。
