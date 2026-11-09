## 1. RED

- [x] 1.1 增加 OpenSpec proposal/design/spec/verification。
- [x] 1.2 先写 fake app-server 子进程测试，覆盖成功流、RPC 错误与 context cancellation/interrupt。
- [x] 1.3 在 Go 1.27.3 容器内确认测试因生产实现缺失而 RED。

## 2. GREEN

- [x] 2.1 实现 JSON-RPC 逐行协议、pending response 路由与 notification channel。
- [x] 2.2 实现 app-server 子进程启动、initialize/initialized 与 stderr drain。
- [x] 2.3 实现串行 `RunTurn`、thread/start、turn/start、delta/completed 与 turn/interrupt。

## 3. Verification

- [x] 3.1 相关测试通过并达到新增包 80% 以上 statement coverage。
- [x] 3.2 相关测试在 race detector 下通过。
- [x] 3.3 `openspec validate add-codex-appserver-provider --strict` 与 `git diff --check` 通过。
- [x] 3.4 记录未执行真实 Codex/OpenAI live smoke 的边界。

