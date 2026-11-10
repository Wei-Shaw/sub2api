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

## 4. Official app-server black-box E2E

- [x] 4.1 增加 build-tag 黑盒测试，真实启动固定版本官方 `codex app-server` 并连接 loopback fake Responses API。
- [x] 4.2 增加固定基础镜像、非 root 运行的测试 Dockerfile。
- [x] 4.3 增加 `--network none`、只读 rootfs、drop capabilities 的一键脚本。
- [x] 4.4 实际执行黑盒测试，验证真实请求的 fake Bearer、model、stream、input，以及 app-server delta/completed。
- [x] 4.5 验证临时 `CODEX_HOME` 不含 `auth.json`，记录没有真实账号、OpenAI 网络或额度消耗。

## 5. User HTTP E2E and warm worker pool

- [x] 5.1 实现按账号隔离、容量有界、idle 回收的常驻 worker pool。
- [x] 5.2 实现 session key → thread ID 粘性，并覆盖同会话复用和 pool busy/exhausted。
- [x] 5.3 解析 `thread/tokenUsage/updated.last` 并映射到既有 usage 结果。
- [x] 5.4 为显式 app-server 账号增加 Responses JSON/SSE service adapter，其他账号路径不变。
- [x] 5.5 增加真实 HTTP → handler → 账号选择 → pool → 官方 app-server → fake upstream 黑盒 E2E，连续两次请求验证单进程和 thread 复用。
- [x] 5.6 复用现有 OpenAI account scheduler；将单 worker app-server 账号的调度并发/load factor 收口为 1。
- [x] 5.7 pool 达到容量时优先 LRU 淘汰 idle worker，全 active 时才 exhausted。
