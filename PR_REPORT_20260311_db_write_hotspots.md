# PR Report: DB 写入热点与后台查询拥塞排查

## 背景

线上在高峰期出现了几类明显症状：

- 管理后台仪表盘接口经常超时，`/api/v1/admin/dashboard/snapshot-v2` 一度达到 50s 以上
- 管理后台充值接口 `/api/v1/admin/users/:id/balance` 出现 15s 以上超时
- 登录态刷新、扣费、错误记录在高峰期出现大量 `context deadline exceeded`
- PostgreSQL 曾出现连接打满，后续回退连接池后，主问题转为 WAL/刷盘拥塞

本报告基于 `/home/ius/sub2api` 当前源码，目标是给出一份可直接拆成 PR 的修复方案。

## 结论

这次故障的主因不是单一“慢 SQL”，而是请求成功路径上的同步写库次数过多，叠加部分后台查询仍直接扫 `usage_logs`，最终把 PostgreSQL 的 WAL 刷盘、热点行更新和 outbox 重建链路一起放大。

代码层面的核心问题有 6 个。

### 1. 成功请求路径同步写库过多

`backend/internal/service/gateway_service.go:6594` 的 `postUsageBilling` 在单次请求成功后，可能同步触发以下写操作：

- `userRepo.DeductBalance`
- `APIKeyService.UpdateQuotaUsed`
- `APIKeyService.UpdateRateLimitUsage`
- `accountRepo.IncrementQuotaUsed`
- `deferredService.ScheduleLastUsedUpdate`（这一项已经做了延迟批量，是正确方向）

也就是说，一次成功请求不是 1 次落库，而是 3 到 5 次写入。

这和线上看到的现象是吻合的：

- `UPDATE accounts SET extra = ...`
- `INSERT INTO usage_logs ...`
- `INSERT INTO ops_error_logs ...`
- `scheduler_outbox` backlog

### 2. API Key 配额更新存在额外读写放大

`backend/internal/service/api_key_service.go:815` 的 `UpdateQuotaUsed` 当前流程是：

1. `IncrementQuotaUsed`
2. `GetByID`
3. 如超限再 `Update`

对应仓储实现：

- `backend/internal/repository/api_key_repo.go:441` 只做自增
- 然后 service 再回表读取完整 API Key
- 之后可能再整行更新状态

这让“每次扣费后更新 API Key 配额”从 1 条 SQL 变成了最多 3 次数据库交互。

### 3. `accounts.extra` 被当成高频热写字段使用

两个最重的热点都落在 `accounts.extra`：

- `backend/internal/repository/account_repo.go:1159` `UpdateExtra`
- `backend/internal/repository/account_repo.go:1683` `IncrementQuotaUsed`

问题有两个：

1. 两者都会重写整块 JSONB，并更新 `updated_at`
2. `UpdateExtra` 每次写完都会额外插入一条 `scheduler_outbox`

尤其 `UpdateExtra` 现在被多处高频调用：

- `backend/internal/service/openai_gateway_service.go:4039` 持久化 Codex rate-limit snapshot
- `backend/internal/service/ratelimit_service.go:903` 持久化 OpenAI Codex snapshot
- `backend/internal/service/ratelimit_service.go:1013` / `1025` 更新 session window utilization

这类“监控/额度快照”并不会改变账号是否可调度，却仍然走了：

- JSONB 更新
- `updated_at`
- `scheduler_outbox`

这是明显的写放大。

### 4. `scheduler_outbox` 设计偏向“每次状态变更都写一条”，高峰期会反压调度器

`backend/internal/repository/scheduler_outbox_repo.go:79` 的 `enqueueSchedulerOutbox` 非常轻，但它被大量调用。

例如：

- `UpdateExtra` 每次都 enqueue `AccountChanged`
- `BatchUpdateLastUsed` 也会 enqueue 一条 `AccountLastUsed`
- 各类账号限流、过载、错误状态切换也都会 enqueue

对应的 outbox worker 在：

- `backend/internal/service/scheduler_snapshot_service.go:199`
- `backend/internal/service/scheduler_snapshot_service.go:219`

它会不断拉取 outbox，再触发 `GetByID`、`rebuildBucket`、`loadAccountsFromDB`。

所以当高频写入导致 outbox 增长时，系统不仅多了写，还会反向带出更多读和缓存重建。

### 5. 仪表盘只有一部分走了预聚合，`models/groups/users-trend` 仍然直接扫 `usage_logs`

好消息是，`dashboard stats` 本身已经接了预聚合表：

- `backend/internal/repository/usage_log_repo.go:306`
- `backend/internal/repository/usage_log_repo.go:420`
- 预聚合表定义在 `backend/migrations/034_usage_dashboard_aggregation_tables.sql:1`

但后台慢的不是只有 stats。

`snapshot-v2` 默认会同时拉：

- stats
- trend
- model stats

见：

- `backend/internal/handler/admin/dashboard_snapshot_v2_handler.go:68`

其中：

- `GetUsageTrendWithFilters` 只有“无过滤、day/hour”时才走预聚合，见 `usage_log_repo.go:1657`
- `GetModelStatsWithFilters` 直接扫 `usage_logs`，见 `usage_log_repo.go:1805`
- `GetGroupStatsWithFilters` 直接扫 `usage_logs`，见 `usage_log_repo.go:1872`
- `GetUserUsageTrend` 直接扫 `usage_logs`，见 `usage_log_repo.go:1101`
- `GetAPIKeyUsageTrend` 直接扫 `usage_logs`，见 `usage_log_repo.go:1046`

所以线上会出现：

- stats 快
- 但 `snapshot-v2` 仍然慢
- `/admin/dashboard/users-trend` 单独也慢

这和你线上看到的日志完全一致。

### 6. 管理后台充值是“读用户 -> 整体更新用户 -> 插审计记录”

`backend/internal/service/admin_service.go:694` 的 `UpdateUserBalance` 当前流程：

1. `GetByID`
2. 在内存里改 balance
3. `userRepo.Update`
4. `redeemCodeRepo.Create` 记录 admin 调账历史

而 `userRepo.Update` 是整用户对象更新，并同步 allowed groups 事务处理：

- `backend/internal/repository/user_repo.go:118`

这个接口平时不一定重，但在数据库已经抖动时，会比一个原子 `UPDATE users SET balance = balance + $1` 更脆弱。

## 额外观察

### `ops_error_logs` 虽然已异步化，但单条写入仍然很重

错误日志中间件已经做了队列削峰：

- `backend/internal/handler/ops_error_logger.go:69`
- `backend/internal/handler/ops_error_logger.go:106`

这点方向是对的。

但落库表本身很重：

- `backend/internal/repository/ops_repo.go:23`
- `backend/migrations/033_ops_monitoring_vnext.sql:69`
- `backend/migrations/033_ops_monitoring_vnext.sql:470`

`ops_error_logs` 不仅列很多，还带了多组 B-Tree 和 trigram 索引。高错误率时，即使改成异步，也还是会把 WAL 和 I/O 压上去。

## 建议的 PR 拆分

建议拆成 4 个 PR，不要在一个 PR 里同时改数据库模型、后台查询和管理接口。

### PR 1: 收缩成功请求路径的同步写库次数

目标：把一次成功请求的同步写次数从 3 到 5 次，尽量压到 1 到 2 次。

建议改动：

1. 把 `APIKeyService.UpdateQuotaUsed` 改为单 SQL
   - 新增 repo 方法，例如 `IncrementQuotaUsedAndMaybeExhaust`
   - 在 SQL 里同时完成 `quota_used += ?` 和 `status = quota_exhausted`
   - 返回 `key/status/quota/quota_used` 最小字段，直接失效缓存
   - 删掉当前的 `Increment -> GetByID -> Update`

2. 把账号 quota 计数从 `accounts.extra` 拆出去
   - 最理想：新增结构化列或独立 `account_quota_counters` 表
   - 次优：至少把 `quota_used/quota_daily_used/quota_weekly_used` 从 JSONB 中剥离

3. 对“纯监控型 extra 字段”禁止 enqueue outbox
   - 例如 codex snapshot、session_window_utilization
   - 这些字段不影响调度，不应该触发 `SchedulerOutboxEventAccountChanged`

4. 复用现有 `DeferredService` 思路
   - `last_used` 已经是批量刷盘，见 `deferred_service.go:41`
   - 可继续扩展 `deferred quota snapshot flush`

预期收益：

- 直接减少 WAL 写入量
- 降低 `accounts` 热点行锁竞争
- 降低 outbox 增长速度

### PR 2: 给 dashboard 补齐预聚合/缓存，避免继续扫 `usage_logs`

目标：后台仪表盘接口不再直接扫描大表。

建议改动：

1. 为 `users-trend` / `api-keys-trend` 增加小时/天级预聚合表
2. 为 `model stats` / `group stats` 增加日级聚合表
3. `snapshot-v2` 增加分段缓存
   - `stats`
   - `trend`
   - `models`
   - `groups`
   - `users_trend`
   避免一个 section miss 导致整份 snapshot 重新扫库
4. 可选：把 `include_model_stats` 默认值从 `true` 改成 `false`
   - 至少让默认仪表盘先恢复可用，再按需加载重模块

预期收益：

- `snapshot-v2`
- `/admin/dashboard/users-trend`
- `/admin/dashboard/api-keys-trend`

这几类接口会从“随数据量线性恶化”变成“近似固定成本”。

### PR 3: 简化管理后台充值链路

目标：管理充值/扣余额不再依赖整用户对象更新。

建议改动：

1. 新增 repo 原子方法
   - `SetBalance(userID, amount)`
   - `AddBalance(userID, delta)`
   - `SubtractBalance(userID, delta)`

2. `UpdateUserBalance` 改为：
   - 先执行原子 SQL
   - 再读一次最小必要字段返回
   - 审计记录改为异步或降级写

3. 审计记录建议改名或独立表
   - 现在把后台调账记录塞进 `redeem_codes`，语义上不干净

预期收益：

- `/api/v1/admin/users/:id/balance` 在库抖时更稳
- 失败面缩小，不再被 allowed groups 同步事务拖累

### PR 4: 为重写路径增加“丢弃策略”和“熔断指标”

目标：高峰期先保护主链路，不让非核心写入拖死数据库。

建议改动：

1. `ops_error_logs`
   - 增加采样或分级开关
   - 对重复 429/5xx 做聚合计数而不是逐条插入
   - 对 request body / headers 存储加更严格开关

2. `scheduler_outbox`
   - 增加 coalesce/merge 机制
   - 同一账号短时间内多次 `AccountChanged` 合并为一条

3. 指标补齐
   - outbox backlog
   - ops error queue dropped
   - deferred flush lag
   - account extra write QPS

## 推荐实施顺序

1. 先做 PR 1
   - 这是这次线上故障的主链路
2. 再做 PR 2
   - 解决后台仪表盘慢
3. 再做 PR 3
   - 解决后台充值接口脆弱
4. 最后做 PR 4
   - 做长期保护

## 验证方案

每个 PR 合并前都建议做同一组验证：

1. 压测成功请求链路，记录单请求 SQL 次数
2. 观测 PostgreSQL：
   - `pg_stat_activity`
   - `pg_stat_statements`
   - `WALWrite` / `WalSync`
   - 每分钟 WAL 增量
3. 观测接口：
   - `/api/v1/auth/refresh`
   - `/api/v1/admin/dashboard/snapshot-v2`
   - `/api/v1/admin/dashboard/users-trend`
   - `/api/v1/admin/users/:id/balance`
4. 观测队列：
   - `ops_error_logs` queue length / dropped
   - `scheduler_outbox` backlog

## 可直接作为 PR 描述的摘要

This PR reduces database write amplification on the request success path and removes several hot-path writes from `accounts.extra` + `scheduler_outbox`. It also prepares dashboard endpoints to rely on pre-aggregated data instead of scanning `usage_logs` under load. The goal is to keep admin dashboard, balance update, auth refresh, and billing-related paths stable under sustained 500+ RPS traffic.
