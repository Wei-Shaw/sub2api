# OpenAI reserve active overflow Task 5 发版演练检查清单

## 本地回归证据

- RED：`go test -tags unit ./internal/service -run "TestReserveSemanticShift_.*|TestOpenAIGatewayService_(PreviousResponseReserveAcceptedForAny|PreviousResponseReserveAcceptedForActive|SelectAccountWithScheduler_PreviousResponseReserveRejectedForAny|SelectAccountWithScheduler_PreviousResponseReserveRejectedForActive|SelectAccountWithScheduler_ReserveSharedStickyBindingRejectedForAny|SelectAccountWithScheduler_ReserveSharedStickyBindingRejectedForActive|.*GPT55.*)|TestSelectByLoadBalance_(TargetGroupAnyCanSelectReserve|TargetGroupActiveCanSelectReserve|TargetGroupAnyNeverSelectsReserve|TargetGroupActiveNeverSelectsReserve|ReserveSelectedGroupWritesReserveForActiveAny|.*GPT55.*|.*Reserve.*(ProjectionMiss|UnknownModelMiss|CacheNotReady))|Test(Sticky|PreviousResponse|WSContinuation).*Reserve.*|TestLegacyReserveBindingWithExhaustedAffinityDomainStillReadable" -count=1`
- RED 结果：`TestSelectByLoadBalance_GPT55ProjectionTreatsCaseAndSysAsEquivalent/active_uppercase` 失败，旧断言期望 `selected_group=active`，实际为 `reserve`。
- GREEN：更新旧断言并补齐 Task 5 伞状回归后，同一 reserve gate 通过。
- GREEN：`go test -tags unit ./internal/service -run "TestUsageAndOps_AnyReserveCarriesRequestIDAndRoutingSnapshotToUsage" -count=1` 通过，覆盖 `request id -> routing snapshot -> usage` 的 `any -> reserve` 链路。
- GREEN：`go test -tags integration ./internal/repository -run "TestOpsRepositoryListRequestDetails_ReserveRoutingOnlyIncludesAnyTarget" -count=1` 通过，覆盖 request details 中 `any -> reserve` 的 request id 与路由字段。
- GREEN：`go test ./internal/handler -count=1` 通过。
- GREEN：`go test -tags integration ./internal/repository -count=1` 通过。
- GREEN：`go build ./cmd/server` 通过。
- GREEN：`git diff --check` 通过。
- CONCERN：`go test -tags unit ./internal/service -count=1` 当前仍失败在非本任务链路 `TestOpenAIGatewayService_OpenAIPassthrough_429And529TriggerFailover/oauth_429_rate_limit`，单测复现同样在 `RateLimitService.handle429 -> openAIPassthroughFailoverRepo.ClearRateLimit` 空指针崩溃；未修改该无关基线红灯。

## 本地发版演练检查清单

1. 不做真实 VPS 部署；本任务只保留本地 gate 与预发布检查证据。
2. 若后续发版包含前端或 embed 构建，先执行 `pnpm build`，再执行 `go build -tags embed`，避免嵌入旧 `dist`。
3. 从 Windows 构建 Linux VPS 候选时，显式设置 `GOOS=linux`、`GOARCH=amd64`、`CGO_ENABLED=0`。
4. 上传前记录候选二进制时间戳与大小，上传到 `/tmp/sub2api-linux-amd64.new` 这类临时路径，不直接覆盖正式二进制。
5. 在 VPS 上用 `file` 确认候选是 Linux x86_64 ELF。
6. 使用正式 env 启动只绑定 `127.0.0.1:18081` 的临时实例。
7. 预发布 smoke 至少执行 `curl http://127.0.0.1:18081/health`。
8. reserve 语义 smoke 记录三条 request id：plain `gpt-5.5` active/any、`GPT-5.5-Sys`、`GPT-5.4-Sys`。
9. 每条 smoke 核验 `routing_target_group` 保持请求语义，`routing_selected_group=reserve`，selected account 来自当前 canonical projection 的 `ReserveOverflowIDs`，projection metadata 完整。
10. 额外 smoke 旧 binding：`selected_group=reserve + affinity_domain=exhausted` 在 active/any/exhausted 下可读，新写入统一为 `affinity_domain=reserve`，`previous_response` 与 WS continuation 都验证一次。
11. 正式切换后再次复测第 7-10 项，再观察 request details、usage 与 ops breakdown 中 `any -> reserve` 未丢失。
