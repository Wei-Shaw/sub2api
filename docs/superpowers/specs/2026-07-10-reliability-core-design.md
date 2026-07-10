# Sub2API P0 可靠性内核设计

状态：已复核，实施中
方案批准：2026-07-10，用户批准“方案 A：模块化单体渐进硬化”
执行仓库：`D:\sub2api-trunk`
执行分支：`wujie/video-capture-moat-20260702`

## 1. 背景

Sub2API 已经具备内部 AI API、Provider 调度、视频任务、计费、内容采集与本地资产归档能力。当前最主要的架构风险不是功能缺失，而是视频任务终态、事件、扣费、内容采集和资产归档分别执行，缺少一个可恢复、可审计的事务边界。

已核验的当前行为：

- 视频终态先更新 `video_tasks`，随后写 `video_task_events`、`video_usage_logs`、余额扣减、内容采集和本地归档。
- 事件写入失败会向上返回错误，辅助记录可能反向影响已经成立的任务状态。
- `balance_charged_at` 在实际扣余额前写入；进程在两步之间退出时可能留下“标记已扣、余额未扣”。
- 视频创建只受可选的单次费用上限保护，没有用户级并发余额预留。
- 内容采集与归档保持 fail-open 是正确的主链策略，但目前没有 durable retry，失败后可能永久丢失。
- 普通网关请求已有 `UsageBillingRepository.Apply` 原子扣费基础，但财务明细与业务使用日志仍不是同一不可变账本。

本设计是总体改造的第一个子项目，只处理可靠性内核。Provider 出网策略、逐 Key production capability、Google/标准 API Key 鉴权统一、v2/v3 canonical contract、前端经营查询模型和大文件拆分分别在后续独立 spec 中实施。

## 2. 目标

1. 让视频任务状态、事件、用量、余额结算在进程崩溃和重复投递下保持一致。
2. 在真实 Provider 调用前原子预留用户余额，防止并发任务过度承诺同一余额。
3. 建立不可变、可幂等的财务交易账本；`usage_logs` 与 dashboard 作为可重建投影。
4. 让内容采集、资产归档、缓存刷新和通知进入可重试 Outbox，不阻塞用户主链。
5. 保持现有 HTTP API、任务状态枚举、QCanvas v2/v3 调用行为和管理控制台向后兼容。
6. 保持模块化单体，不新增独立服务、消息队列或外部基础设施。

## 3. 非目标

- 不在本阶段启用或调用真实付费 Provider。
- 不修改 QCanvas 仓库。
- 不实现多租户、公开 SaaS、完整 SkillEngine 或剪辑台。
- 不在本阶段拆分 `gateway_service.go`、`openai_gateway_service.go` 等大文件。
- 不删除 `balance_charged_at`、legacy 视频字段或现有 `usage_logs`。
- 不改变现有 API 对任务 `queued/submitted/running/succeeded/failed/cancelled` 的公开状态语义。
- 不 push、deploy、reset、clean、rebase 或读取 `.env`、Key、token、cookie。

## 4. 强制不变量

### 4.1 任务不变量

- 同一个任务只能从当前允许状态迁移到下一个状态；持久化必须使用 `version` CAS。
- 已进入 terminal 的任务不能被事件、归档、采集或通知失败改写为另一个 terminal 状态。
- Provider 已接受但本地未确认的提交不得自动再次创建付费任务。
- 相同终态结果重复处理不会重复写事件、重复扣费或重复创建副作用任务。

### 4.2 财务不变量

- 本 spec 新增的 flag-on 视频路径中，每次余额变动必须对应一条不可变 `billing_transactions` 记录；普通网关、支付与退款路径在后续子项目迁入同一账本。
- `transaction_key` 唯一，视频结算固定使用 `video_task:<task-id>:charge`；重复结算返回原结果，同时允许未来用不同 key 追加多条 adjustment/refund。
- 余额预留与任务创建在同一事务中完成。
- 余额结算或释放与 reservation 状态变化在同一事务中完成。
- 金额落库使用 `NUMERIC/DECIMAL`；Go 边界复用仓库现有 `github.com/shopspring/decimal v1.4.0` 定义明确的 `Money` 类型，不在新接口中继续传播裸 `float64`。
- `users.balance` 已是 `DECIMAL(20,8)`，本阶段把它作为 legacy 兼容投影；flag-on 视频事务直接以 decimal SQL 读取和更新，不调用仍暴露 `float64` 的旧 `UserRepository.DeductBalance`。不可变 `billing_transactions` 是新路径的财务真相；投影固定 round-half-up 到 8 位，账本 USD 金额保留 10 位。
- 每条交易固化币种、原币金额、USD 结算金额、汇率、汇率时间、定价来源和定价版本。

### 4.3 副作用不变量

- 内容采集、资产归档、缓存刷新和通知失败不得改变主任务终态或财务结算。
- Outbox 事件至少一次投递；消费者必须幂等。
- Outbox 达到最大重试次数后进入 `dead`，不能被静默删除；控制台/日志必须可见。

## 5. 总体架构

```text
CreateTask
  -> transaction: lock user + calculate available balance
  -> insert billing_reservation
  -> insert video_task(api_key_id, reservation_id, version=1)

SubmitTask
  -> CAS queued -> submitted/dispatching
  -> provider call
  -> transaction: persist upstream_task_id + dispatch state + event
  -> ambiguous network outcome: mark dispatch_unknown, never auto-create again

Poll terminal
  -> FinalizeVideoTask transaction
       1. CAS task version/status
       2. insert terminal event
       3. insert video_usage_logs idempotently
       4. settle/release reservation
       5. insert billing_transactions idempotently
       6. update user balance and balance_charged_at when chargeable
       7. enqueue capture/archive/cache/notification outbox events
  -> commit

DomainOutboxWorker
  -> claim with FOR UPDATE SKIP LOCKED + lease
  -> idempotent consumer
  -> complete / retry with backoff / dead
```

## 6. 数据模型

下一迁移号使用 `156`，但实现计划允许把非事务索引拆成 `157_*_notx.sql`。

### 6.1 `video_tasks` 增量字段

| 字段 | 类型 | 语义 |
|---|---|---|
| `api_key_id` | `BIGINT NULL` | API-key 视频任务的 credential 归因；管理员创建可为空 |
| `creation_key` | `VARCHAR(64) NULL` | 规范化 `Idempotency-Key` 的 SHA-256 哈希；新 flag-on 任务非空，partial unique |
| `creation_fingerprint` | `VARCHAR(64) NULL` | canonical create payload 哈希；同 key 不同 payload 判冲突 |
| `reservation_id` | `BIGINT NULL` | 对应余额预留 |
| `version` | `BIGINT NOT NULL DEFAULT 1` | 状态 CAS 版本 |
| `dispatch_state` | `VARCHAR(24) NOT NULL DEFAULT 'pending'` | `pending/dispatching/accepted/unknown/not_required` |
| `settlement_status` | `VARCHAR(24) NOT NULL DEFAULT 'pending'` | `pending/settled/released/not_required/error` |
| `archive_status` | `VARCHAR(24) NOT NULL DEFAULT 'pending'` | `pending/succeeded/failed/not_required` |
| `capture_status` | `VARCHAR(24) NOT NULL DEFAULT 'pending'` | `pending/succeeded/failed/not_required` |

旧行回填规则：

- 成功且 `balance_charged_at IS NOT NULL`：`settlement_status='settled'`。
- 失败/取消：`settlement_status='not_required'`。
- 已有 `local_asset_path`：`archive_status='succeeded'`。
- 其他历史行保持 `pending`，由只读 reconciliation 扫描器判断，不在 migration 中触发扣费或网络下载。

### 6.2 `billing_reservations`

关键字段：

- `id`
- `reservation_key`，唯一。flag-on HTTP 创建固定从稳定 creation key 派生为 `video_task:create:<creation-key>`，不得为每次重试生成新随机值。
- `source_type`、`source_id`
- `user_id`、可空 `api_key_id`
- `reserved_amount_usd NUMERIC(20,10)`
- `settled_amount_usd NUMERIC(20,10)`
- `status`：`active/settled/released/expired/review_required`
- `expires_at`、`created_at`、`updated_at`、`settled_at`、`released_at`

创建 reservation 时先 `SELECT users ... FOR UPDATE`，再计算：

```text
available = users.balance - SUM(active reservations for user)
```

余额无法确定、余额不足或 reservation 查询失败时，真实 Provider 创建 fail-closed。Mock 任务不创建 reservation。

`creation_key` 规则：优先复用调用者的原始 `Idempotency-Key`，使用现有 `NormalizeIdempotencyKey` 与 `HashIdempotencyKey`，数据库只存 hash；请求未提供时由服务生成原始 key，并仅在 `Idempotency-Key` 响应头和可选 `idempotency_key` 响应字段返回，后续请求仍只存 hash。只有调用者重试时复用同一原始 key 才承诺跨请求幂等。相同 key、相同 payload 并发创建返回同一 task/reservation；相同 key、不同 payload 返回 `IDEMPOTENCY_KEY_CONFLICT`。

### 6.3 `billing_transactions`

关键字段：

- `id`
- `transaction_key`，全局唯一
- `source_type`：`gateway_request/video_task/payment/refund`
- `source_id`
- `transaction_kind`：`charge/release/refund/adjustment`
- `user_id`、可空 `api_key_id/account_id/subscription_id/reservation_id`
- `amount_original`、`currency_original`
- `amount_usd`
- `exchange_rate`、`exchange_rate_as_of`
- `pricing_source`、`pricing_version`
- `balance_before`、`balance_after`
- `metadata JSONB`
- `created_at`

唯一约束：`transaction_key`。另建 `(source_type, source_id, created_at)` 查询索引，不限制同一 source 后续追加 adjustment/refund。

账本记录不可 update/delete。纠错使用新的 `adjustment/refund` 交易反向记账。

### 6.4 `domain_outbox`

关键字段：

- `id`
- `aggregate_type`、`aggregate_id`
- `event_type`
- `dedup_key UNIQUE`
- `payload JSONB`
- `status`：`pending/processing/completed/dead`
- `attempt_count`
- `next_attempt_at`
- `locked_at`、`locked_until`、`locked_by`
- `last_error`
- `created_at`、`completed_at`

首批事件类型：

- `video.capture_content`
- `video.archive_asset`
- `billing.invalidate_cache`
- `billing.notify_low_balance`

Outbox 不复用 `scheduler_outbox`。后者是调度快照专用、以外部 watermark 消费；领域 Outbox 需要逐条完成状态、重试、dead-letter 和 lease，生命周期不同。

首版运行参数固定为：

- poll interval：1 秒。
- claim batch：50 条。
- lease：2 分钟。
- 最大尝试次数：8 次。
- 重试退避：`5s, 10s, 20s, 40s, 80s, 160s, 300s`，上限 5 分钟，不加随机抖动以保证测试确定性。
- 第 8 次失败后写入 `dead`，保留 `last_error` 的脱敏摘要。

这些值必须进入 typed config 并带校验；首版默认值固定，不能从环境变量另建平行配置入口。

Reservation reaper 默认每 60 秒执行一次，也进入同一 typed config。它只允许用 CAS 原子释放 `queued + dispatch_state=pending + expires_at<=now`；`submitted/running/dispatch_state=unknown` 只能原子标记为 `review_required`。释放/标记、审计 transaction 与必要 Outbox 必须同事务；两个 reaper 并发只能应用一次。

## 7. 组件边界

### 7.1 Service 层

- `video_task_finalization.go`
  - 定义 `VideoTaskFinalizationInput`、`VideoTaskFinalizationResult`。
  - 只负责校验 terminal 语义和调用 repository transaction。
- `billing_money.go`
  - 定义 Money、币种与 decimal 转换边界。
- `domain_outbox_worker.go`
  - 通用 claim/retry/dead-letter 生命周期。
- `billing_reservation_reaper.go`
  - 回收未 dispatch 的过期预留，并把已 dispatch/unknown 的过期预留送入人工复核。
- `video_outbox_handlers.go`
  - 注册内容采集与资产归档消费者。
- `video_gateway_worker.go`
  - 只负责 provider submit/poll 编排；terminal 后调用 finalizer，不再直接扣费、采集或归档。

### 7.2 Repository 层

- `video_task_finalization_repo.go`
  - 拥有跨 `video_tasks/video_task_events/video_usage_logs/users/billing_* /domain_outbox` 的单事务写入。
- `billing_reservation_repo.go`
  - 任务创建 reservation、释放和查询。
- `billing_transaction_repo.go`
  - 追加不可变交易、按 source 查询。
- `domain_outbox_repo.go`
  - claim、complete、retry、reap expired lease。

现有 `video_gateway_repo.go` 保留 CRUD/查询；随着任务落地，终态与结算 SQL 迁移到新 repository 文件，但仍属于同一 Go package，不引入循环依赖。

### 7.3 Composition Root

- 所有依赖必须写入 `backend/internal/service/wire.go`、`backend/internal/repository/wire.go` 的 ProviderSet。
- 禁止手改 `wire_gen.go`。
- 重新生成 Wire 后必须以 `git diff --exit-code backend/cmd/server/wire_gen.go` 验证可复现。

## 8. 状态与错误处理

### 8.1 Provider submit 模糊结果

以下场景判定为 `dispatch_state='unknown'`：

- POST 已发送后发生网络超时或连接中断，无法证明 Provider 未创建任务。
- Provider 返回成功但本地事务无法保存 `upstream_task_id`。

处理规则：

- 不自动再次调用 create。
- 任务公开状态继续兼容 `submitted`，同时在详情返回 `dispatch_state` 和可读 `next_action`。
- 如果 Provider 支持 client request id 查询，reconciliation 使用内部 task ID 查询并恢复；不支持时进入人工复核队列。

### 8.2 Terminal finalization

- CAS 失败且数据库已经是相同 terminal 状态：返回 idempotent success。
- CAS 失败且 terminal 状态冲突：记录高优先级错误，不覆盖数据库真相。
- 结算事务失败：整个 terminal transaction 回滚，worker 下一轮重新 poll/finalize。
- Outbox handler 失败：主任务和结算保持成功，事件进入重试。
- archive/capture handler 可重试失败时对应 task 状态保持 `pending`；handler 成功与 Outbox complete 同事务改为 `succeeded`；第 8 次失败进入 dead 时才与 Outbox dead 同事务改为 `failed`。重放使用 dedup/CAS，不允许状态回退。

Terminal event 的幂等不新增 event dedup 列：只有成功执行 terminal CAS 的事务才能插入 terminal event；CAS 0 行的幂等重放直接返回现有结果，不再次插入事件。

### 8.3 Reservation 差额

- `actual <= reserved`：结算 actual，释放差额。
- `actual > reserved`：仍按实际费用结算，交易 metadata 写 `reservation_overrun=true`，产生告警 Outbox；不回滚已交付任务。
- failed/cancelled：reservation 全额释放，不产生 charge 交易。

## 9. API 兼容

现有字段和状态保持不变，新增字段均可选：

- `dispatch_state`
- `settlement_status`
- `archive_status`
- `capture_status`
- `delivery_status`
- `next_action`

`delivery_status` 是 handler 根据 task、archive 与 result URL 状态派生的只读字段，不新增数据库列。取值固定为 `processing/archiving/deliverable/delivery_failed`。

`local_asset_available` 和 `result_url` 继续保留。前端后续改造优先读取新状态，但旧客户端不读取也不受影响。

## 10. 可观测性

至少增加以下结构化指标：

- `video_finalization_total{status}`
- `video_finalization_conflict_total`
- `billing_reservation_active_total`
- `billing_reservation_overrun_total`
- `billing_settlement_retry_total`
- `domain_outbox_pending_total{event_type}`
- `domain_outbox_dead_total{event_type}`
- `domain_outbox_oldest_age_seconds`
- `video_dispatch_unknown_total{provider}`

所有日志携带 `task_id`、`user_id`、可空 `api_key_id`、`reservation_id`、`transaction_id` 和 request ID；不得写入 Provider Key、用户 API Key 或完整敏感 URL query。

## 11. TDD 与故障注入

实现必须遵守 RED → GREEN → REFACTOR，每个行为先看到测试因缺少能力而失败。

### 11.1 Repository 集成测试

- 同一 terminal finalize 调用两次只扣一次、只有一条 charge transaction。
- 在 task/event/usage/ledger/balance/outbox 任一步注入 SQL 错误，事务不留下部分状态。
- 两个并发 finalize 只有一个成功应用，另一个返回幂等结果。
- 两个并发 reservation 不能共同消费同一可用余额。
- failed/cancelled 释放 reservation，不扣余额。
- actual 超出 reserved 时记录 overrun，余额与账本一致。

### 11.2 Worker 单元测试

- terminal event 持久化失败不把 succeeded 改成 failed。
- capture/archive handler 失败不改变 terminal 状态。
- provider create 模糊结果不触发第二次 create。
- 过期 outbox lease 可被重新 claim；未过期 lease 不可抢占。
- dead-letter 达阈值后停止自动重试并保留错误摘要。

### 11.3 前端契约测试

- `succeeded + archive pending` 显示“生成完成，归档中”。
- `result_url` 过期但 local asset 可用时仍显示可交付。
- `settlement_status=error` 只显示账务待处理，不把生成结果改成失败。
- `dispatch_state=unknown` 显示“已提交，需确认上游任务”，不提供重复创建动作。

## 12. 迁移与发布顺序

1. 先加表、字段、约束和只读 repository，不切主链。
2. 增加 reservation/finalization/outbox 测试并完成新实现。
3. 通过配置开关 `reliability_core.video_enabled` 在本地 mock 环境切换视频路径。
4. 双写观察阶段：旧字段继续写，新账本作为审计对照；发现差异立即关闭开关。
5. 普通 mock-only 浏览器路径验证 create → task ID → worker → terminal → Outbox → archive → preview，并断言无 reservation/charge；独立无网络 `billableFakeAdapter` integration harness 验证 reservation → ledger → 幂等扣费。
6. 完成一轮 reconciliation，确认无重复扣费和未结算成功任务。
7. 只有另行获得真实 Provider 授权、预算和停止条件后，才允许最小真实验证；不属于本 spec 自动执行内容。

配置开关 `reliability_core.video_enabled` 默认 `false`。只有 mock-only 集成测试、migration schema test、reconciliation dry-run 和全量门禁全部通过后，才可在本地开发配置中显式设为 `true`；本 spec 不改变生产默认值。

Reservation 默认有效期为 6 小时。清理器只能自动释放仍处于 `queued` 且没有 provider dispatch 证据的过期 reservation；`submitted/running/dispatch_state=unknown` 对应 reservation 只能标记 `review_required`，禁止自动释放。

验收拆成两条完全离线的证明：普通 Mock 路径断言 reservation/charge 均不存在，只验证任务、finalization、Outbox、归档与预览；本地 `billableFakeAdapter` 使用固定虚拟价格验证 reservation、ledger、扣费与重放幂等，但不发起任何网络请求。

## 13. 回滚

- 关闭 `reliability_core.video_enabled` 即回到旧编排路径。
- 新表和新增字段保留，不做 destructive down migration。
- 新增交易账本不可删除；测试/开发错误数据通过 adjustment 记录纠正。
- 代码回滚使用 `git revert`，不使用 `reset --hard`、`clean` 或 rebase。
- 如果新路径已产生 reservation，回滚前先运行只读 reconciliation，再用显式 release 命令释放仍 active 且对应任务已 terminal 的 reservation。

## 14. 验收标准

- 后端 `go test ./... -count=1` 通过。
- 后端 `golangci-lint run ./...` 通过。
- 前端 `npx.cmd eslint . --ext .ts,.vue --max-warnings=0` 通过。
- 前端 `npx.cmd vue-tsc --noEmit` 通过。
- 前端 `npx.cmd vitest run --reporter=basic` 通过。
- secret scan 通过，不读取或打印凭证。
- `git diff --check` 通过。
- mock-only 用户路径产生真实本地 task ID，任务进入 terminal，余额/账本/reservation 一致，结果可预览或下载，失败原因可见。
- 故障注入证明事件、采集、归档或进程重启不会重复扣费或污染 terminal 状态。
- 更新 `docs/reviews/LATEST_REVIEW_PACKAGE.html`，包含命令、结果、风险、回滚、文件索引和下一步提示词。

## 15. 后续独立子项目

本 spec 完成后按顺序进入：

1. P0 hardened egress 与 caller-scoped production capability。
2. P1 标准/Google API Key 鉴权内核统一。
3. P1 canonical video spec、图片 usage normalizer 与 Money DTO。
4. P1 前端 Query/Money/错误与任务生命周期。
5. P2 Gateway/Settings/Usage Repository 结构拆分、Wire 可复现和机器可读契约。

每个子项目单独设计、计划、TDD、审查和提交，不同时修改互相重叠的核心文件。
