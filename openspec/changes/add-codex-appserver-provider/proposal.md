## Why

sub2api 当前通过 Go 代码直接构造 OpenAI/Codex 上游请求。为了评估官方 `codex app-server` 作为独立执行后端的可行性，需要先建立一个小而可测试的 provider 基础：真实启动 stdio 子进程、完成 JSON-RPC 初始化、启动 thread/turn、转发事件，并在调用取消时发送 `turn/interrupt`。

## What Changes

- 新增独立的 `backend/internal/codexapp` 包，封装 app-server 子进程与 JSON-RPC 生命周期。
- 新增单 worker `Provider`，支持一次一个 turn 的文本输入、事件回调和取消。
- 新增基于测试子进程的 hermetic E2E，真实覆盖 stdin/stdout JSONL、事件流和 interrupt，不访问 OpenAI。
- 保持现有 OpenAI direct forwarding 路径不变；本变更不接入账号调度、OAuth、计费或生产路由。

## Capabilities

### New Capabilities

- `codex-appserver-provider`：官方 Codex app-server 的最小进程协议客户端与可重复本地 E2E 基础。

## Impact

- **后端**：新增隔离包，不修改现有 handler、service、repository 或数据库。
- **运行时**：只有显式构造 provider 时才启动子进程；当前生产启动链不会调用它。
- **测试**：使用当前 Go test binary 充当 fake app-server，不需要 Codex 登录、OpenAI 凭据或网络。

