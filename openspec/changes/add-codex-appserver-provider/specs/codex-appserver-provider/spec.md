## Purpose

为 sub2api 提供一个可独立验证的 Codex app-server provider 基础，使后续生产接入能够复用同一套进程协议实现，而不继续在 OpenAI gateway 内复制 AgentLoop。

## ADDED Requirements

### Requirement: Provider 完成 app-server 初始化握手

系统 SHALL 启动配置的子进程，通过 stdin/stdout JSONL 调用 `initialize`，使用明确的第三方 `clientInfo`，成功后发送 `initialized`。初始化失败 MUST 关闭子进程并返回带上下文的错误。

#### Scenario: 初始化成功

- **WHEN** fake app-server 对 `initialize` 返回成功响应
- **THEN** provider MUST 发送 `initialized` 后允许创建 thread

### Requirement: Provider 执行一个串行 turn 并转发事件

系统 SHALL 在缺少既有 thread ID 时调用 `thread/start`，随后调用 `turn/start`，并把匹配 turn 的文本 delta 和 completed 状态交给调用方。首版 MUST 串行执行同一 provider 上的 turn。

#### Scenario: 文本流完成

- **WHEN** app-server 返回 thread/turn 并发送文本 delta 与 `turn/completed`
- **THEN** 调用方 MUST 收到 delta，结果 MUST 包含真实 thread ID、turn ID 和 completed 状态

#### Scenario: turn/start 返回 RPC 错误

- **WHEN** app-server 对 `turn/start` 返回 JSON-RPC error
- **THEN** provider MUST 返回包含 method、code 和 message 的错误，不得伪造 completed

### Requirement: Provider 以本轮 token usage 接入计费结果

系统 SHALL 读取匹配 thread/turn 的 `thread/tokenUsage/updated`，并累加各 notification 的 `tokenUsage.last` 作为本轮输入、缓存输入、输出、推理输出和总 token；MUST NOT 把累计 `tokenUsage.total` 重复计费。

#### Scenario: 本轮 usage 到达 completed 之前

- **WHEN** app-server 发送匹配 turn 的 token usage notification 后发送 `turn/completed`
- **THEN** `TurnResult.Usage` MUST 等于 notification 的 `last` usage

### Requirement: 调用取消通过 turn/interrupt 传播

worker 生命周期 MUST 独立于单次调用 context。收到 context cancellation 后，provider SHALL 在短超时内调用 `turn/interrupt`，并向调用方返回原 context 错误。

#### Scenario: 调用方取消活动 turn

- **WHEN** `turn/start` 已成功且调用 context 被取消
- **THEN** fake app-server MUST 观察到匹配 thread/turn 的 `turn/interrupt`
- **THEN** provider MUST 返回 `context.Canceled`

### Requirement: Hermetic E2E 使用真实子进程边界

测试 SHALL 启动独立 helper process 并通过 OS pipe 交换 JSONL，不得仅 mock Go interface。测试 MUST NOT 需要网络、OAuth token 或真实 OpenAI 服务。

#### Scenario: 离线测试

- **WHEN** 在无 Codex 登录和无网络依赖的环境运行相关测试
- **THEN** 成功、RPC 错误和取消场景 MUST 可重复完成

### Requirement: 官方 app-server 黑盒测试不得访问真实账号或公网

系统 SHALL 提供可选黑盒测试，运行固定版本官方 Codex app-server，并将 custom Responses provider 指向测试容器 loopback fake upstream。测试运行阶段 MUST 断开容器网络、使用空临时 `CODEX_HOME` 与假 token，MUST NOT 挂载宿主凭据目录。

#### Scenario: 官方 binary 连接 fake Responses upstream

- **WHEN** 黑盒测试通过 `Provider.RunTurn` 启动真实 app-server turn
- **THEN** fake upstream MUST 收到 POST `/v1/responses`、假 Bearer token、目标 model、`stream:true` 与非空 input
- **THEN** provider MUST 收到由真实 app-server 转换出的文本 delta 与 completed 状态

#### Scenario: 运行时网络和凭据隔离

- **WHEN** 执行黑盒测试容器
- **THEN** 容器 MUST 使用 `--network none`、只读 rootfs、非 root 用户和临时写目录
- **THEN** 临时 `CODEX_HOME` MUST 不含 `auth.json`
- **THEN** 测试 MUST NOT 需要或读取宿主 Codex/OpenAI 登录态

### Requirement: worker pool 按账号常驻复用并按会话粘住 thread

系统 SHALL 以账号 key 隔离 app-server worker，并在 idle TTL 内复用已初始化进程。提供 session key 时 SHALL 将成功 turn 的 thread ID 用于同 session 后续请求；没有 session key 的请求 SHALL 使用 ephemeral thread。pool MUST 有最大 worker 数量，所有容量都忙时 MUST 快速返回 exhausted。

worker pool MUST NOT 自行选择账号或复制现有账号负载均衡。账号负载、slot、等待与换号 SHALL 继续由现有 OpenAI account scheduler 管理。当前单 worker 串行实现中，app-server 账号的 scheduler concurrency 和 load factor MUST 为 1。

#### Scenario: 同账号同会话连续两次请求

- **WHEN** 两次 turn 使用相同 worker key 和 session key
- **THEN** factory MUST 只创建一个 worker
- **THEN** 第二次 turn MUST 携带第一次结果的 thread ID

#### Scenario: pool 已满且 worker 忙

- **WHEN** 新账号请求到达且 pool 所有容量均被活动 turn 占用
- **THEN** 请求 MUST 收到 pool exhausted 错误，不得无界启动新进程

#### Scenario: pool 已满但存在 idle worker

- **WHEN** 新账号需要 worker，pool 达到上限且至少一个 worker idle
- **THEN** pool MUST 淘汰最久未使用的 idle worker并为新账号建 worker

#### Scenario: app-server 账号配置了更高并发

- **WHEN** app-server 账号的持久化 `Concurrency` 或 `LoadFactor` 大于 1
- **THEN** OpenAI scheduler MUST 仍按实际串行能力 1 计算 load 并获取 slot

### Requirement: 显式账号可走用户 HTTP app-server 路径

系统 SHALL 仅对 `openai_execution_backend=codex_app_server` 的账号启用 app-server service adapter；其他账号 MUST 保持现有 direct forwarding。适配器 SHALL 接受 Responses 文本输入，输出非流式 Responses JSON 或流式 Responses SSE，并把本轮 usage 交给现有 `OpenAIForwardResult`。

#### Scenario: 用户经 Responses handler 连续请求

- **WHEN** 用户携带有效测试 API key 向 `/openai/v1/responses` 连续发送同会话请求
- **THEN** 请求 MUST 经账号选择和常驻 pool 到达真实官方 app-server
- **THEN** 用户 MUST 收到 `completed` Responses 输出和 fake upstream 返回的文本
- **THEN** 两次请求 MUST 只创建一个 app-server 进程并续接同一 thread
