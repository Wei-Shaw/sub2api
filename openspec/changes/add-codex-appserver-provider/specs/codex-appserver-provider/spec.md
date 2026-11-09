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
