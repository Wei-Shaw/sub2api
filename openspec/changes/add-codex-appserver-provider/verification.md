## Planned Evidence

- Go 1.27.3 容器中的 RED 输出：测试成功编译到新增 API 引用处，并因实现缺失失败。
- GREEN 输出：成功 turn 收到 delta/completed；上游 RPC error 原样映射；取消触发 fake 进程观察到 `turn/interrupt`。
- 新增包 coverage 与 race 结果。
- OpenSpec strict 校验与 diff whitespace 校验。

## RED Evidence

- `docker exec -w /src/backend/backend sub2api-go-test-40 go test ./internal/codexapp -count=1`
- 结果：退出码 1；测试包成功进入编译，因预期缺少 `Event`、`TurnRequest`、`InputItem`、`RPCError`、`Provider` 等生产 API 而失败。
- 边界：此前一次 bind mount 在当前 Docker daemon 上只得到空目录，`go.mod not found` 属于环境失败，不计入 RED；改用 `docker cp` 后才取得上述有效 RED。

## Acceptance Boundary

本变更验证两层：快速层覆盖 sub2api provider 与 fake app-server 子进程；可选黑盒层覆盖 provider、固定版本官方 Codex binary、真实 app-server JSON-RPC 和容器 loopback fake Responses API。两层都不访问 ChatGPT/OpenAI 账号或模型服务，不证明 OAuth、模型权限、配额、OpenAI 边缘层、模型语义或服务端风控。黑盒入口仍是 `codexapp.Provider`，尚未覆盖生产 `/v1/responses` handler、账号路由与计费。

## GREEN Evidence

- `go test ./internal/codexapp -count=1`（Go 1.27.3 容器）：通过。
- `go test -coverprofile=/tmp/codexapp.cover ./internal/codexapp -count=1`：通过，statement coverage `81.5%`。
- `go test -race ./internal/codexapp -count=1`：通过。
- `go vet ./internal/codexapp`：通过。
- `golangci-lint v2.14.0 run ./internal/codexapp --timeout=5m`：`0 issues`。v2.14.0 来自当前 `.github/workflows/backend-ci.yml`；旧 `DEV_GUIDE.md` 所写 v2.13.0 无法读取 Go 1.27.3 export data，不作为代码失败。
- `go test -tags=unit ./... -timeout=10m`：完整后端 unit 通过，含 `internal/service`（约 249 秒）。
- `openspec validate add-codex-appserver-provider --strict`：通过。

## Official binary black-box evidence

- `backend/scripts/test-codex-appserver-blackbox.sh`
- 构建：固定 `golang:1.27.3-bookworm`、`node:22-bookworm-slim` digest 与 `@openai/codex@0.166.0`。
- 运行约束：`--network none`、`--read-only`、`--cap-drop ALL`、`no-new-privileges`、非 root、无宿主 volume。
- 结果：`TestRealAppServerAgainstFakeResponses` 通过，耗时 9.40 秒，退出码 0。
- 观察：官方 app-server 向 loopback `/v1/responses` 发出 POST；携带 `Bearer fake-codex-e2e-token`；body 中 model 为 `gpt-6-sol`、`stream:true`、input 非空；provider 收到 hello delta 与 completed；临时 `CODEX_HOME` 中不存在 `auth.json`。
- 未发生：没有读取宿主 `~/.codex`/钥匙串/token，没有访问 OpenAI/ChatGPT 域名，没有真实账号请求或额度消耗。

## Debugging Evidence

- 首次 race 全量失败均发生在 helper cleanup 的 1 秒 deadline。单独复现仍固定约 1.02 秒，根因是 race helper 的默认退出等待与 cleanup deadline 相等。
- 仅给测试 helper 增加 `GORACE=atexit_sleep_ms=0` 后，race 通过；生产关闭时限未放宽。
