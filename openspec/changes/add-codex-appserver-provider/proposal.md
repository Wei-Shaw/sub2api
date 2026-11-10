## Why

sub2api 当前通过 Go 代码直接构造 OpenAI/Codex 上游请求。为了评估官方 `codex app-server` 作为独立执行后端的可行性，需要先建立一个小而可测试的 provider 基础：真实启动 stdio 子进程、完成 JSON-RPC 初始化、启动 thread/turn、转发事件，并在调用取消时发送 `turn/interrupt`。

## What Changes

- 新增独立的 `backend/internal/codexapp` 包，封装 app-server 子进程与 JSON-RPC 生命周期。
- 新增单 worker `Provider`，支持一次一个 turn 的文本输入、事件回调和取消。
- 新增基于测试子进程的 hermetic E2E，真实覆盖 stdin/stdout JSONL、事件流和 interrupt，不访问 OpenAI。
- 新增断网容器黑盒测试：运行固定版本官方 `codex app-server`，通过 custom provider 指向容器 loopback fake Responses API，验证真实官方 binary 生成的请求与事件链路。
- 新增按账号隔离的常驻 worker pool、会话到 thread 的粘性映射、idle 回收和容量上限；HTTP 请求不按次启动 app-server。
- 为显式配置 `openai_execution_backend=codex_app_server` 的账号增加 Responses service 适配，复用现有 handler 的鉴权上下文、账号调度与计费记录链；其他账号的 direct forwarding 路径不变。
- 新增用户视角黑盒测试：真实 HTTP 请求进入 `/openai/v1/responses`，经 handler、账号选择、常驻 pool 和官方 app-server 到达 fake Responses upstream，再返回兼容 Responses 输出。

## Capabilities

### New Capabilities

- `codex-appserver-provider`：官方 Codex app-server 的最小进程协议客户端与可重复本地 E2E 基础。

## Impact

- **后端**：新增隔离包与窄 service 接口，不修改 repository 或数据库。
- **运行时**：只有显式配置 app-server backend 的账号且注入 pool 后才使用新路径；当前生产启动链尚未创建 pool，因此不会自动启动子进程。
- **测试**：快速测试使用当前 Go test binary 充当 fake app-server；可选黑盒脚本在构建阶段安装固定版本官方 Codex，运行阶段使用 `--network none`、空临时 `CODEX_HOME` 和假 token，不需要 Codex 登录或 OpenAI 凭据。
