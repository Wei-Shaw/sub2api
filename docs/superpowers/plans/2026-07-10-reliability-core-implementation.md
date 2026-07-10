# Sub2API P0 Reliability Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` to execute this plan task by task. Each task must follow `superpowers:test-driven-development`; production implementation may start only after the named RED test fails for the expected missing behavior.

**Goal:** 在不触发真实 Provider、不改变现有公开任务状态和不引入外部基础设施的前提下，为视频任务建立可回滚的余额预留、原子终态结算、不可变账本和可重试领域 Outbox，并让前端正确表达交付生命周期。

**Architecture:** 保持 Go 模块化单体。`VideoGatewayWorker` 继续负责 Provider 编排；新的 finalization repository 拥有任务终态、事件、用量、reservation、ledger、余额和 outbox 的单一 PostgreSQL 事务；领域 Outbox worker 负责 fail-open 副作用。所有新主链由默认关闭的 `reliability_core.video_enabled` 控制，旧主链保留为回滚路径。

**Tech Stack:** Go 1.24、Gin、PostgreSQL、go-redis、Wire、`shopspring/decimal v1.4.0`、Vue 3、TypeScript、Pinia、Vitest、Vue Test Utils、Playwright（仅 mock-only 验收）。

**Design source:** `docs/superpowers/specs/2026-07-10-reliability-core-design.md`

**Execution root:** `D:\sub2api-trunk`

## Global constraints

- 不读取或打印 `.env`、API Key、token、cookie、JWT 或账号密码。
- 不调用真实付费 Provider；测试仅使用内存 fake、`httptest.Server` 或既有 mock provider。
- 不 push、deploy、reset、clean、rebase；不修改或删除用户已有的 `.worktrees/`。
- 每个任务开始前执行 `git status --short`、`git branch --show-current`、`git rev-parse --show-toplevel`；只提交本任务文件。
- 所有金额新边界使用 `decimal.Decimal` 或本计划定义的 `Money`，不得新增裸 `float64` 财务接口。
- 禁止手改 `backend/cmd/server/wire_gen.go`；依赖变化后必须通过 Wire 命令重新生成。
- 每个任务按 RED → GREEN → REFACTOR 执行，记录 RED/GREEN 命令与结果到 `.superpowers/sdd/task-*/implementer-report.md`。
- 每个任务实现提交后，必须通过独立 reviewer 的规格符合性与代码质量审查；未通过不得进入下一任务。
- Task 1 开始前把实际 `git rev-parse HEAD` 记录为 `.superpowers/sdd/progress.md` 的 `START_HEAD`；Task 9 的 whole-branch review 必须使用该值，不得硬编码历史 SHA。

**Mandatory execution order:** Task 1 → Task 2 → Task 3 → Task 6 → Task 4 → Task 5 → Task 7 → Task 8 → Task 9。Task 6 的 dispatch 幂等与 reservation reaper 必须在 terminal/outbox 主链切换前成立。

---

## Task 1: Freeze schema, typed configuration, and money boundaries

**Purpose:** 建立向后兼容的数据与配置地基，不切换运行时主链。

**Files:**

- Create: `backend/migrations/156_reliability_core.sql`
- Modify: `backend/internal/repository/migrations_schema_integration_test.go`
- Create: `backend/internal/service/billing_money.go`
- Create: `backend/internal/service/billing_money_test.go`
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/config/config_test.go`
- Modify: `deploy/config.example.yaml`

### 1.1 RED — schema contract

- [ ] 先在 schema integration test 断言 `video_tasks` 九个新增字段（含 `creation_key/creation_fingerprint`），三张新表，以及 creation/reservation/transaction/outbox 唯一键。
- [ ] 运行 `go test -tags=integration ./internal/repository -run TestMigrationsRunner_IsIdempotent_AndSchemaIsUpToDate -count=1`，确认因缺少 migration 156 而失败。

### 1.2 GREEN — additive migration

- [ ] 创建幂等 `156_reliability_core.sql`；使用 `ADD COLUMN IF NOT EXISTS`、`CREATE TABLE IF NOT EXISTS`、幂等 constraint/index block。
- [ ] 实现 spec 的完整字段、状态 CHECK、外键、`NUMERIC(20,10)` 和索引。
- [ ] 历史任务只做 spec 允许的确定性回填；migration 不扣费、不释放 reservation、不下载资产。
- [ ] 重跑 schema test；若本机 integration DB 不可用，记录外部 gate，并至少通过 migration parser 与普通 repository tests。

### 1.3 RED/GREEN — typed config

- [ ] 先写 `TestLoadDefaultReliabilityCoreConfig`：默认关闭、poll=1s、batch=50、lease=120s、max attempts=8、reservation TTL=6h、reservation reaper interval=60s、退避 `[5,10,20,40,80,160,300]` 秒。
- [ ] 先写 table-driven validation：负值、零 lease、`len(backoff) != maxAttempts - 1`、退避非递增均拒绝。
- [ ] 在 `Config` 增加 `ReliabilityCore ReliabilityCoreConfig mapstructure:"reliability_core"`，内部定义 `VideoEnabled`、`ReservationTTLHours`、`ReservationReapIntervalSeconds`、`Outbox DomainOutboxConfig`。
- [ ] 只通过 Viper typed config 设置默认值和校验；同步 `deploy/config.example.yaml`，保持 `video_enabled: false`。

### 1.4 RED/GREEN — Money

- [ ] 先覆盖 USD 构造、10 位小数 round-trip、不同币种不能相加、负 reservation 拒绝、JSON/DB string 不经过 float。
- [ ] 实现 `Currency`、`Money`、`NewMoney`、`MustUSD`（仅测试/常量）、`Add`、`Subtract`、`Compare`、`IsNegative`、`Decimal`、`String`；生产输入错误返回 error。
- [ ] 本阶段不迁移 `users.balance DECIMAL(20,8)`；新 flag-on 视频 repository 必须直接用 decimal SQL 读写并 round-half-up 到 8 位，不调用旧的 float64 `DeductBalance`。ledger 保留 10 位并作为新路径财务真相。
- [ ] 运行：

```powershell
cd backend
go test ./internal/config ./internal/service -run 'Test(LoadDefaultReliabilityCoreConfig|ReliabilityCoreConfig|Money)' -count=1
go test ./internal/repository -count=1
```

### 1.5 Commit

- [ ] `git diff --check`
- [ ] 仅 stage 本任务七个文件。
- [ ] `git commit -m "feat(reliability): add billing and outbox foundations"`

---

## Task 2: Add repository primitives for reservations, immutable ledger, and outbox

**Purpose:** 提供可组合但尚未接管 worker 的可靠数据访问原语。

**Files:**

- Create: `backend/internal/service/reliability_core_types.go`
- Create: `backend/internal/repository/billing_reservation_repo.go`
- Create: `backend/internal/repository/billing_reservation_repo_integration_test.go`
- Create: `backend/internal/repository/billing_transaction_repo.go`
- Create: `backend/internal/repository/billing_transaction_repo_integration_test.go`
- Create: `backend/internal/repository/domain_outbox_repo.go`
- Create: `backend/internal/repository/domain_outbox_repo_integration_test.go`
- Modify: `backend/internal/repository/wire.go`

### 2.1 Exact contracts

- [ ] 在 service types 定义 `BillingReservation`、`BillingTransaction`、`DomainOutboxEvent`；金额字段为 `Money`，payload 为 `json.RawMessage`，可空关系为指针。
- [ ] 定义最小的 `BillingReservationRepository`、`BillingTransactionRepository`、`DomainOutboxRepository` 接口。事务内 `*sql.Tx` helper 留在 repository package，不泄漏给 service。

### 2.2 RED/GREEN — reservation

- [ ] integration test 先证明两个并发 reservation 不能共同消费同一余额；余额未知/查询错误 fail-closed；同 `reservation_key` 重放不重复占用。
- [ ] 事务内 `SELECT users.balance ... FOR UPDATE`，查询 active reservation，计算 available，插入或返回同 key 记录；全程使用 decimal。
- [ ] 提供只读 `GetByID/GetByKey/ListExpired`，本任务不实现自动释放策略。

### 2.3 RED/GREEN — ledger

- [ ] integration test 先覆盖同 `transaction_key` 幂等、不同 adjustment key 可追加、10 位精度、按 source 查询顺序。
- [ ] repository 仅实现 `Append` 与查询，不提供 Update/Delete；唯一键重放读取原交易，字段冲突返回 typed conflict。

### 2.4 RED/GREEN — outbox lease

- [ ] integration test 先覆盖按 `next_attempt_at` claim、`FOR UPDATE SKIP LOCKED` 并发不重复、lease 抢占规则、complete 幂等、第 8 次 dead、脱敏错误摘要。
- [ ] 实现 `Enqueue/ClaimBatch/Complete/Retry/ReapExpiredLeases/Counts`；repository 持久化 worker 给出的 next-attempt/dead 决策。
- [ ] 运行：

```powershell
cd backend
go test -tags=integration ./internal/repository -run 'Test(BillingReservation|BillingTransaction|DomainOutbox)' -count=1
go test ./internal/repository ./internal/service -count=1
```

### 2.5 Wire and commit

- [ ] 将三个 concrete constructor 加入 `repository.ProviderSet`；不启动 worker。
- [ ] `git diff --check`
- [ ] `git commit -m "feat(reliability): add reservation ledger and outbox repositories"`

---

## Task 3: Create video tasks with API-key attribution and balance reservation

**Purpose:** 真实 Provider 路径在任务落库前原子预留余额；mock 路径保持零财务副作用。

**Files:**

- Modify: `backend/internal/service/video_gateway_types.go`
- Modify: `backend/internal/handler/video_handler.go`
- Modify: `backend/internal/service/video_gateway_service.go`
- Modify: `backend/internal/service/video_gateway_billing.go`
- Create: `backend/internal/repository/video_task_creation_repo.go`
- Create: `backend/internal/repository/video_task_creation_repo_integration_test.go`
- Modify: `backend/internal/repository/video_gateway_repo.go`
- Modify: `backend/internal/repository/wire.go`
- Modify: `backend/internal/server/routes/api_key_video_gateway_test.go`
- Modify: `backend/internal/service/video_gateway_billing_test.go`
- Modify: `backend/internal/service/wire.go`

### 3.1 RED/GREEN — credential attribution

- [ ] API-key route test 先断言创建参数携带 `middleware.GetAPIKeyFromContext(c).ID`，admin/JWT 路径为 nil。
- [ ] 给 create params/task 增加可空 `APIKeyID`、`ReservationID` 及 version/dispatch/settlement/archive/capture 字段，更新 scan/insert/select。
- [ ] 删除 handler 内三份重复参数组装，提取一个私有 mapper；provider 分支只设置策略 flag。
- [ ] handler 复用现有 `NormalizeIdempotencyKey`/`HashIdempotencyKey`：数据库只存稳定 key hash 和 canonical payload fingerprint；缺 header 时生成原始 key，仅在 `Idempotency-Key` 响应头与可选 `idempotency_key` 响应字段返回，后续重试仍只存 hash。

### 3.2 RED/GREEN — atomic task + reservation

- [ ] integration test 先覆盖成功同事务、余额不足不落库且不 dispatch、任一步失败无半成品、mock 无 reservation、并发不超订、相同 creation key 同 payload 返回同一 task/reservation、相同 key 不同 payload typed conflict。
- [ ] 增加 `VideoTaskCreationRepository.CreateWithReservation(ctx, VideoTaskCreationInput)`；reservation key 固定为 `video_task:create:<creation-key>`，锁 user、写 reservation、写 task、回填引用。
- [ ] 从现有 `VideoPricingCatalog`、`estimateVideoCost`、`calculateVideoActualCost` 抽取返回 `Money + PricingSnapshot` 的窄接口；creation/finalization 共用该口径，禁止平行价格算法。
- [ ] flag false 走旧 creation；flag true 且真实 Provider 才走新事务。任何创建错误发生在 dispatch 前。
- [ ] 运行：

```powershell
cd backend
go test ./internal/server/routes -run 'TestAPIKeyVideoGateway.*Create' -count=1
go test ./internal/service -run 'TestVideoGateway.*(Billing|Create|Reservation)' -count=1
go test -tags=integration ./internal/repository -run TestVideoTaskCreation -count=1
```

### 3.3 Commit

- [ ] `git diff --check`
- [ ] `git commit -m "feat(video): reserve balance when creating provider tasks"`

---

## Task 4: Make terminal finalization one idempotent transaction

**Purpose:** 一次事务完成终态、事件、用量、reservation、账本、余额和 outbox，移除“先 claim 后扣费”的崩溃窗口。

**Files:**

- Create: `backend/internal/service/video_task_finalization.go`
- Create: `backend/internal/service/video_task_finalization_test.go`
- Create: `backend/internal/repository/video_task_finalization_repo.go`
- Create: `backend/internal/repository/video_task_finalization_repo_integration_test.go`
- Modify: `backend/internal/service/video_gateway_worker.go`
- Modify: `backend/internal/service/video_gateway_worker_test.go`
- Modify: `backend/internal/service/video_gateway_types.go`
- Modify: `backend/internal/service/video_gateway_billing.go`
- Modify: `backend/internal/repository/video_gateway_repo.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/internal/repository/wire.go`

### 4.1 RED — service semantics

- [ ] 定义 input：task ID、expected version、terminal status、provider result/error、actual duration/resolution/tokens、actual cost `Money`、pricing metadata、completed time。
- [ ] 单元测试先覆盖非 terminal 拒绝、相同终态幂等、冲突终态 typed conflict、failed/cancelled 不 charge、succeeded 只调用一次 repository。

### 4.2 RED — transaction/fault matrix

- [ ] integration subtests 分别在 task CAS、event、usage、reservation、ledger、balance、outbox 处失败，验证整个事务无部分状态。
- [ ] finalize 两次只产生一条 terminal event、一条 usage、一条 `video_task:<id>:charge`、一组 dedup outbox，余额只减一次。
- [ ] 并发 finalize 一个 applied、一个 idempotent；冲突 terminal 不覆盖已存真相。
- [ ] actual<=reserved 结算 actual 并释放差额；overrun 写 metadata 并 enqueue warning；failed/cancelled 全额 release 且无 charge。

### 4.3 GREEN — transaction and worker switch

- [ ] CAS 使用 expected version 与 non-terminal status，成功时 version+1；0 行后读取当前 task，区分幂等与冲突。
- [ ] terminal event 只在成功 CAS 的同一事务内插入；CAS 0 行的幂等重放直接返回，不再次插 event。video usage、billing transaction、outbox 继续依赖各自唯一键；用户 balance 与 `balance_charged_at` 同事务。
- [ ] flag-on terminal 分支只调用 `VideoTaskFinalizer.Finalize`，不再直接串行 Update/Event/Usage/Billing/Capture/Archive。
- [ ] finalization 失败回滚并允许 worker 重试；flag-off 保留旧路径。
- [ ] 运行：

```powershell
cd backend
go test ./internal/service -run 'Test(VideoTaskFinal|VideoGatewayWorker)' -count=1
go test -tags=integration ./internal/repository -run TestVideoTaskFinalization -count=1
go test ./internal/service ./internal/repository -count=1
```

### 4.4 Commit

- [ ] `git diff --check`
- [ ] `git commit -m "feat(video): finalize tasks and billing atomically"`

---

## Task 5: Run durable domain outbox side effects

**Purpose:** 内容采集、资产归档、缓存刷新和通知失败可重试，但不污染任务与财务真相。

**Files:**

- Create: `backend/internal/service/domain_outbox_worker.go`
- Create: `backend/internal/service/domain_outbox_worker_test.go`
- Create: `backend/internal/service/video_outbox_handlers.go`
- Create: `backend/internal/service/video_outbox_handlers_test.go`
- Modify: `backend/internal/service/generation_content.go`
- Modify: `backend/internal/service/video_asset_archive.go`
- Modify: `backend/internal/service/video_gateway_worker.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/cmd/server/wire.go`
- Generate: `backend/cmd/server/wire_gen.go`

### 5.1 RED/GREEN — deterministic worker lifecycle

- [ ] fake clock/repo tests 先覆盖 1s poll、batch 50、lease 2m、固定 backoff、第 8 次 dead、graceful shutdown、panic 转 retry、未知事件 non-retryable。
- [ ] 日志错误必须脱敏截断，不包含 payload URL query 或 credential-like 字段。
- [ ] 实现 worker：claim 后按 registry 处理，成功 Complete，失败按 typed config Retry/Dead。

### 5.2 RED/GREEN — idempotent handlers

- [ ] capture 重放只产生一个结果；archive 在 `local_asset_path` 已存在时 no-op；cache/notification 使用 dedup key。
- [ ] capture/archive 可重试错误只让 outbox retry，对应 task 状态保持 `pending`；成功时状态 `succeeded` 与 Outbox complete 同事务；第 8 次失败时状态 `failed` 与 Outbox dead 同事务。状态更新使用 dedup/CAS，重放不得回退。
- [ ] 每条事件进入 dead 时写一条脱敏结构化 error 日志，包含 event type、aggregate/task ID、attempt count 和安全错误摘要；不得包含 payload、URL query 或 credential。
- [ ] 适配层复用现有 collector、archive service、billing cache，不复制脱敏或下载逻辑。
- [ ] flag-on 删除同步 side effects；flag-off 保留旧路径。

### 5.3 Wire reproducibility

- [ ] 接入 server lifecycle；flag false 时 worker 不 claim。
- [ ] 修改 `wire.go`/provider sets 后在 `backend` 运行 `go generate ./cmd/server`，禁止手改 `wire_gen.go`。
- [ ] 记录第一次生成 diff，并把生成文件纳入本任务提交；提交后再运行 `go generate ./cmd/server`，以 `git diff --exit-code -- cmd/server/wire_gen.go` 证明可复现。若第二次仍产生 diff，修复后 amend 本任务提交并重复验证。
- [ ] 运行：

```powershell
cd backend
go test ./internal/service -run 'Test(DomainOutbox|VideoOutbox)' -count=1
go test ./cmd/server ./internal/service ./internal/repository -count=1
```

### 5.4 Commit

- [ ] `git diff --check`
- [ ] `git commit -m "feat(reliability): process video side effects through outbox"`

---

## Task 6: Guard provider dispatch with CAS and ambiguity-safe states

**Purpose:** Provider 可能已接受但本地未确认时，绝不自动重复创建付费任务。

**Files:**

- Modify: `backend/internal/service/video_gateway_worker.go`
- Modify: `backend/internal/service/video_gateway_worker_test.go`
- Modify: `backend/internal/service/video_gateway_types.go`
- Modify: `backend/internal/repository/video_gateway_repo.go`
- Modify: `backend/internal/repository/video_gateway_repo_test.go`
- Create: `backend/internal/service/video_dispatch_reconciliation.go`
- Create: `backend/internal/service/video_dispatch_reconciliation_test.go`
- Create: `backend/internal/service/billing_reservation_reaper.go`
- Create: `backend/internal/service/billing_reservation_reaper_test.go`
- Modify: `backend/internal/repository/billing_reservation_repo.go`
- Modify: `backend/internal/repository/billing_reservation_repo_integration_test.go`
- Modify: `backend/internal/service/wire.go`

### 6.1 RED — state transitions

- [ ] tests 先覆盖：queued/pending 只 claim 一次；接受并保存 upstream ID 后 accepted；POST 后 timeout/EOF 或保存 ID 失败后 unknown；unknown 重跑 create 次数仍为 1；CAS 冲突不覆盖别的 worker。

### 6.2 GREEN — explicit CAS methods

- [ ] 新路径使用 `MarkDispatchingCAS`、`MarkDispatchAcceptedCAS`、`MarkDispatchUnknownCAS`，每次迁移 version+1，状态与 event 同事务。
- [ ] `ListRunnableTasks` 排除 unknown；公开 status 兼容 submitted。
- [ ] `VideoDispatchReconciler.DryRun` 只报告 unknown 与建议动作，不调用 provider、不修改 task、不泄漏 URL/credential。

### 6.3 RED/GREEN — reservation reaper

- [ ] tests 先覆盖：过期 queued+pending 自动 release；submitted/running/unknown 只标 `review_required`；未过期不处理；两个 reaper 并发只应用一次；release/ledger/outbox 任一步失败整事务回滚。
- [ ] repository 用 CAS 锁定候选；release/review_required、`video_task:<id>:reservation_expired` 审计 transaction 和通知 Outbox 同事务。
- [ ] `BillingReservationReaper` 每 60 秒运行一次，flag false 时不写；停止时响应 context cancellation，不触发 provider。
- [ ] 运行：

```powershell
cd backend
go test ./internal/service -run 'Test(VideoGatewayWorker.*Dispatch|VideoDispatch)' -count=1
go test ./internal/repository -run 'TestVideoGatewayRepository.*Dispatch' -count=1
go test ./internal/service -run 'TestBillingReservationReaper' -count=1
go test -tags=integration ./internal/repository -run 'TestBillingReservation.*Reap' -count=1
```

### 6.4 Commit

- [ ] `git diff --check`
- [ ] `git commit -m "fix(video): guard dispatch and reap stale reservations"`

---

## Task 7: Expose and render delivery lifecycle without breaking clients

**Purpose:** 后端提供可选可靠性状态，前端以本地资产优先的真实交付状态渲染和轮询。

**Files:**

- Modify: `backend/internal/handler/video_handler.go`
- Modify: `backend/internal/handler/video_handler_c1_contract_test.go`
- Modify: `backend/internal/service/video_gateway_poll_response_contract_test.go`
- Modify: `frontend/src/api/admin/video.ts`
- Create: `frontend/src/composables/useVideoTaskLifecycle.ts`
- Create: `frontend/src/composables/__tests__/useVideoTaskLifecycle.spec.ts`
- Modify: `frontend/src/views/admin/video/VideoTaskDetailView.vue`
- Modify: `frontend/src/views/admin/video/VideoTasksView.vue`
- Modify: `frontend/src/views/admin/video/__tests__/VideoTasksView.spec.ts`
- Create: `frontend/src/views/admin/video/__tests__/VideoTaskDetailView.spec.ts`

### 7.1 RED/GREEN — API compatibility

- [ ] contract tests 先断言旧字段完整保留，并新增可选 dispatch/settlement/archive/capture/delivery/next-action 字段。
- [ ] 后端 table test：non-terminal→processing；success+archive pending（含 retry 中）→archiving；local asset 优先→deliverable；只有 Outbox dead 使 archive failed 且无资产时→delivery_failed；dispatch unknown 不提供重复创建；settlement error 不改变生成状态。
- [ ] 只在统一 response mapper 派生 delivery 状态，不新增 DB 列；create/get/list/admin/API-key 共用 mapper。
- [ ] `next_action` 使用稳定机器枚举，中文文案由前端映射。

### 7.2 RED/GREEN — frontend lifecycle

- [ ] composable tests 先覆盖单 in-flight、退避上限、hidden 暂停/visible 恢复、unmount abort、terminal+archiving 低频继续、deliverable/dead 停止。
- [ ] `useVideoTaskLifecycle` 持有 polling 和 `AbortController`；view 只渲染。
- [ ] local asset 优先既有 download endpoint，remote URL 次选；均无时显示真实 failure。
- [ ] 显示“生成完成，归档中”“本地资产可下载”“账务待处理”“已提交，需确认上游任务”。

### 7.3 Verify and commit

- [ ] 运行：

```powershell
cd backend
go test ./internal/handler ./internal/service -run 'Test.*Video.*(Contract|Delivery|Poll)' -count=1
cd ..\frontend
npx.cmd vitest run src/composables/__tests__/useVideoTaskLifecycle.spec.ts src/views/admin/video/__tests__/VideoTaskDetailView.spec.ts src/views/admin/video/__tests__/VideoTasksView.spec.ts --reporter=basic
npx.cmd vue-tsc --noEmit
```

- [ ] `git diff --check`
- [ ] `git commit -m "feat(video-ui): expose reliable task delivery lifecycle"`

---

## Task 8: Add reconciliation, metrics, and a mock-only reliability proof

**Purpose:** 在启用开关前证明差异可发现、状态可恢复、关键路径可观察。

**Files:**

- Create: `backend/internal/service/reliability_reconciliation.go`
- Create: `backend/internal/service/reliability_reconciliation_test.go`
- Create: `backend/internal/service/reliability_metrics.go`
- Create: `backend/internal/server/routes/video_reliability_mock_e2e_test.go`
- Modify: `backend/internal/service/video_gateway_worker_test.go`
- Modify: `backend/internal/repository/video_task_finalization_repo_integration_test.go`
- Create: `frontend/src/views/admin/video/__tests__/VideoReliabilityFlow.spec.ts`

### 8.1 RED/GREEN — dry-run reconciliation and metrics

- [ ] tests 先覆盖成功未结算、terminal+active reservation、过期 queued/no dispatch、过期 submitted/running/unknown、账本/balance 差异、dead outbox、成功无可交付资产。
- [ ] `DryRun` 只报告 `severity/code/task_id/recommended_action`；自动释放建议仅限 queued+无 dispatch，其余建议 review_required。
- [ ] 使用项目现有 metrics abstraction 注册 spec 指标；若无现成 abstraction，增加小接口与 no-op，不引入外部基础设施。
- [ ] 日志携带可用关联 ID，但绝不包含 URL query 或 credential。

### 8.2 RED/GREEN — mock-only end-to-end proof

- [ ] 普通 Mock harness 显式开启 flag，走 create→真实本地 task ID→mock submit/poll→terminal finalization→outbox→local asset marker；断言 reservation、charge transaction 均不存在且 balance 不变。
- [ ] 独立 `billableFakeAdapter` 使用固定虚拟价格、不发任何网络请求，验证 reservation→terminal ledger/扣费；capture/archive 首次失败、retry 成功，任务始终 succeeded 且 finalization 重放只扣一次。
- [ ] Vitest 真实 router/Pinia 组件流只 mock 本地 API，覆盖创建、轮询、归档中、可下载、失败原因；禁止外网。
- [ ] 使用 Codex in-app Browser 对本地 mock-only 服务执行一次同路径截图验收；浏览器或本地服务依赖不可用时精确记录 gate，不把未运行写成通过。

### 8.3 Commit

- [ ] 运行定向 Go/Vitest mock-only tests，并记录 Browser 截图验收结果。
- [ ] `git diff --check`
- [ ] `git commit -m "test(reliability): prove mock video settlement and delivery"`

---

## Task 9: Full verification, whole-branch review, and review package

**Purpose:** 用可复核证据收口，不把局部通过误写为产品 READY。

**Files:**

- Modify: `docs/goals/03_CURRENT_GOAL.md`
- Replace: `docs/reviews/LATEST_REVIEW_PACKAGE.html`
- Create: `docs/superpowers/codex-handoff/deliverables/2026-07-10-RELIABILITY-CORE-review.md`
- Modify only if evidence requires: implementation/test files from Tasks 1–8

### 9.1 Full gates

- [ ] 后端：

```powershell
cd D:\sub2api-trunk\backend
go test ./... -count=1
golangci-lint run ./...
```

- [ ] 前端：

```powershell
cd D:\sub2api-trunk\frontend
npx.cmd eslint . --ext .ts,.vue --max-warnings=0
npx.cmd vue-tsc --noEmit
npx.cmd vitest run --reporter=basic
```

- [ ] 运行 repo 既有 secret scan/quality gate，只扫描工作树，不读取 `.env` 或系统凭证。
- [ ] `git diff --check`；重跑 Wire 并确认无新增 diff。
- [ ] integration DB、Playwright 或 lint 工具缺失时记录命令、exit code、依赖和未验证范围。

### 9.2 Whole-branch review

- [ ] fresh reviewer 从 `.superpowers/sdd/progress.md` 读取 Task 1 开始前记录的 `START_HEAD`，审查 `START_HEAD..HEAD` 完整 diff，按 security/correctness/performance/maintainability 分类。
- [ ] P0/P1 finding 必须回到对应任务 TDD 修复、重跑验证、再次审查。
- [ ] 核对 flag-off 兼容、migration 增量、ledger 不可变、reservation 不超订、outbox 不污染终态、unknown 不重发。

### 9.3 Self-contained review package

- [ ] `LATEST_REVIEW_PACKAGE.html` 包含目标、背景、repo/branch/HEAD、变更、架构、commit、命令证据、mock-only 闭环、未跑 gate、截图或原因、风险、回滚、文件索引、状态、下一步 prompt。
- [ ] 同步 Markdown deliverable 与 goal；状态只用“内部可用 / 可演示 / 待复核 / 已阻塞 / 已冻结”。
- [ ] 不宣称真实 Provider、生产部署、QCanvas 跨仓或公网路径已验证。

### 9.4 Final commit

- [ ] `git diff --check`；`git status --short` 确认仅保留用户已有 `.worktrees/`。
- [ ] `git commit -m "docs(reliability): publish P0 reliability review package"`
- [ ] 不 push；回报 commit 序列、验证证据、风险和回滚方法。

---

## Definition of done

- [ ] Tasks 1–9 的 implementer/review reports 完整落在 `.superpowers/sdd/`（过程证据可不提交）。
- [ ] 所有已运行 gate 有命令、exit code、摘要；未运行 gate 明确标为待复核或已阻塞。
- [ ] flag 默认关闭；关闭后旧主链可用，开启仅用于本地 mock/integration 证明。
- [ ] 没有真实 Provider 调用、凭证读取、部署、push 或跨仓写入。
- [ ] `docs/reviews/LATEST_REVIEW_PACKAGE.html` 与最终 HEAD/状态一致。
