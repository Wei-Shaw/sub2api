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

本变更验证三层：快速层覆盖 provider、usage 解码、pool 与 service adapter；provider 黑盒层覆盖固定版本官方 Codex binary、真实 app-server JSON-RPC 和容器 loopback fake Responses；用户黑盒层从真实 HTTP socket 进入 `/openai/v1/responses` handler，覆盖测试鉴权上下文、账号选择、usage recording 入口、pool、官方 binary 与 Responses 输出。所有层都不访问 ChatGPT/OpenAI 账号或模型服务，不证明真实 OAuth、模型权限、配额、OpenAI 边缘层、模型语义或服务端风控。生产 worker factory 尚未接线，不能称为部署验收。

## GREEN Evidence

- `go test ./internal/codexapp -count=1`（Go 1.27.3 容器）：通过。
- `go test -tags=unit -cover ./internal/codexapp -count=1`：通过，scheduler/pool 收口后 statement coverage `84.4%`。
- `go test -race ./internal/codexapp -count=1`：通过。
- `go vet ./internal/codexapp`：通过。
- `golangci-lint v2.14.0 run ./internal/codexapp --timeout=5m`：`0 issues`。v2.14.0 来自当前 `.github/workflows/backend-ci.yml`；旧 `DEV_GUIDE.md` 所写 v2.13.0 无法读取 Go 1.27.3 export data，不作为代码失败。
- `go test -tags=unit ./... -timeout=10m`：完整后端 unit 通过，含 `internal/service`（约 249 秒）。
- `openspec validate add-codex-appserver-provider --strict`：通过。

## Official binary black-box evidence

- `backend/scripts/test-codex-appserver-blackbox.sh`
- 构建：固定 `golang:1.27.3-bookworm`、`node:22-bookworm-slim` digest 与 `@openai/codex@0.166.0`。
- 运行约束：`--network none`、`--read-only`、`--cap-drop ALL`、`no-new-privileges`、非 root、无宿主 volume。
- 结果：`TestRealAppServerAgainstFakeResponses` 通过，session fail-closed 收口后的最终重跑耗时 5.65 秒，退出码 0。
- 观察：官方 app-server 向 loopback `/v1/responses` 发出 POST；携带 `Bearer fake-codex-e2e-token`；body 中 model 为 `gpt-6-sol`、`stream:true`、input 非空；provider 收到 hello delta 与 completed；临时 `CODEX_HOME` 中不存在 `auth.json`。
- 未发生：没有读取宿主 `~/.codex`/钥匙串/token，没有访问 OpenAI/ChatGPT 域名，没有真实账号请求或额度消耗。

## User HTTP black-box evidence

- 同一脚本在相同断网容器中运行 `TestOpenAIResponsesViaRealAppServerUserE2E`。
- 结果：通过，session fail-closed 收口后的最终重跑耗时 5.15 秒，退出码 0。
- 用户输入：两次 `POST /openai/v1/responses`，`model=gpt-6-sol`、`stream=false`、相同 `prompt_cache_key=session-alpha`，文本分别要求返回 hello。
- 用户输出：两次 HTTP 200，Responses `status=completed`，`output[0].content[0].text=hello`。
- 进程与会话观察：worker factory 只创建 1 个官方 app-server；第二次 `TurnRequest.ThreadID` 等于第一次 `TurnResult.ThreadID`；fake upstream 收到 2 次带假 Bearer 的 Responses 请求。
- usage 观察：handler 的 SIMPLE MODE usage recording 两次均记录 `tokens=6`，来自 app-server `thread/tokenUsage/updated.last`，不是测试硬编码到 handler result 的零值。
- 边界：测试 API key 校验、账号 repository 和 fake upstream 是隔离夹具；没有数据库持久化、真实 OAuth、真实计费扣款或真实模型语义。

## Scheduler and pool ownership evidence

- RED：容量已满但存在 idle worker 时，新账号请求得到 `ErrPoolExhausted`；app-server 调度并发 helper 尚不存在，service test 编译失败。
- GREEN：`TestPoolEvictsLeastRecentlyUsedIdleWorkerAtCapacity` 证明保留最近使用账号并淘汰 LRU idle 账号；`TestOpenAIAppServerSchedulingConcurrencyIsOne` 证明 app-server 账号即使配置 `Concurrency=12`、`LoadFactor=8`，scheduler 仍使用 1，direct 账号保持原值。
- 代码边界：账号候选过滤、load/queue/error/TTFT 等评分、slot、等待与换号仍在 `openai_account_scheduler.go`；`codexapp.Pool` 只接收已选定的 account worker key。
- 回归：`go test -tags=unit ./internal/codexapp ./internal/service -timeout=10m`、`go test -race -tags=unit ./internal/codexapp -count=1` 与 `go vet ./internal/codexapp ./internal/service` 均通过。

## Stateful session fail-closed evidence

- RED：相同 `prompt_cache_key` 或 `previous_response_id` 被传给另一个账号时，adapter 返回成功且 fake backend 收到新的 turn，证明存在静默丢上下文风险。
- GREEN：`TestOpenAIGatewayForwardCodexAppServerRejectsCrossAccountSessionMove` 要求两种 continuation 都返回 `503 session_pinned_account_unavailable`，并断言 backend 总调用数保持为首次请求的 1。
- 边界：该守卫只防止错误迁移；本变更仍未实现 thread resume、历史 replay 或跨账号无损迁移。
- 回归：完整 `internal/service` unit（264.75 秒）、`go vet ./internal/service`、golangci-lint v2.14.0（0 issues）及两条官方 binary 黑盒均通过。

## Debugging Evidence

- 首次 race 全量失败均发生在 helper cleanup 的 1 秒 deadline。单独复现仍固定约 1.02 秒，根因是 race helper 的默认退出等待与 cleanup deadline 相等。
- 仅给测试 helper 增加 `GORACE=atexit_sleep_ms=0` 后，race 通过；生产关闭时限未放宽。
