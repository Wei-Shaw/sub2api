# 开发清单：经营对账收编

> 对应 `proposal.md` + `design.md`。按顺序执行，每阶段可独立验收。
>
> **状态：已实施完成。** 勾选项为实际落地情况；与计划有出入的地方在文末「实施偏差」逐条说明。

## 1. 数据模型与持久化

- [x] 1.1 新增迁移 `backend/migrations/242_reconciliation_tables.sql`，建 4 张表：`reconciliation_usage_extras`、`reconciliation_upstream_bills`、`reconciliation_account_rules`、`reconciliation_sync_state`
  - 注：早期设计稿写 5 张表，实际收敛为 **4 张**，功能无缺口，详见「实施偏差」。
- [x] 1.2 `reconciliation_upstream_bills` 加唯一索引 `(provider, upstream_request_id)` 与**唯一部分索引** `(matched_usage_log_id) WHERE matched_usage_log_id IS NOT NULL`
- [x] 1.3 `reconciliation_account_rules.account_id` 唯一；**不加** `(provider, external_key)` 唯一约束（A6 令牌允许多账号共用）
- [x] 1.4 新增 `backend/ent/schema/reconciliation_usage_extra.go`（`Mixin(){mixins.TimeMixin{}}` + `entsql.Annotation{Table:...}` + 中文 `.Comment`）
- [x] 1.5 新增 `backend/ent/schema/reconciliation_upstream_bill.go`
- [x] 1.6 新增 `backend/ent/schema/reconciliation_account_rule.go`
- [x] 1.7 新增 `backend/ent/schema/reconciliation_sync_state.go`
- [x] 1.8 跑 `go generate ./ent` 生成 ent 代码
- [x] 1.9 **追加**：`reconciliation_upstream_bills` 增列 `cache_tokens_total INT NOT NULL DEFAULT 0`。上游 A6 可能只回一个**合并的**缓存值（此时分列读/写都是 0），若按分列比对，这类账单会 100% 匹配失败。组合匹配一律以该合计值为准。

## 2. repository 层（构造函数返回 service 侧接口）

- [x] 2.1 `internal/repository/reconciliation_usage_extra_repo.go`：`UpsertBatch`（按 usage_log_id）、`ListCollectedUsageLogIDs`、`ListAccountIDsByRuleKeys`
- [x] 2.2 `internal/repository/reconciliation_upstream_bill_repo.go`：`UpsertBatch`（`ON CONFLICT (provider, upstream_request_id) DO NOTHING`）、`ListStaging`、`FindDirectMatchCandidates`、`FindCompositeMatchCandidates`、`MarkMatched`、`MarkUnmatched`、`ProviderTokenNames`、`CountUnmatched`
- [x] 2.3 `internal/repository/reconciliation_account_rule_repo.go`：`List`、`GetByAccountID`、`Upsert`（version+1）、`Delete`
- [x] 2.4 `internal/repository/reconciliation_sync_state_repo.go`：`Get`、`Set`、`GetMultiple`
- [x] 2.5 错误翻译走 `translatePersistenceError`（三参签名，无类型时传 `nil, nil`）
- [x] 2.6 **追加** `internal/repository/reconciliation_ledger_repo.go`（看板读模型：`Summary` / `Points` / `Rows` / `UsageCountsByAccount`，全部共用同一份分类 CTE）与 `reconciliation_usage_source_repo.go`（下游用量事实源）

## 3. service 层

- [x] 3.1 `internal/service/reconciliation_types.go`：领域类型 + 全部端口 interface
- [x] 3.2 `internal/service/reconciliation_rule_service.go`：规则 CRUD + **账号列表视图（含零调用账号，用当前 groups/account_groups 关系）**
- [x] 3.3 `internal/service/reconciliation_ledger_service.go`：`Summary` / `TimeSeries` / `Requests`——**三者共用同一份分类 CTE**（分类判定下沉到 SQL 的 `reconciliationLedgerCTE`）
- [x] 3.4 `internal/service/reconciliation_a6_client.go`：鉴权头、分页、`quota_per_unit`、`other` 双重编码兼容、单页 3 次重试、16MB 上限、`UseNumber()`
- [x] 3.5 `internal/service/reconciliation_sync_service.go`：用量增量采集（写 extras，含规则+汇率快照）、A6 账单幂等导入、四级匹配、令牌→账号映射（**含历史快照回查，修 Bug 2**）
- [x] 3.6 `internal/service/reconciliation_collector.go`：`New*`（只造对象）+ `ProvideReconciliationCollector`（`SetLeaderLock` + `Start()`）、幂等 `Start`/`Stop`、`runLoop` 用 ticker+ctx
- [x] 3.7 `internal/service/provider_pricing_service.go`：读取价格文档，`site_domain` 改为 `chenshuapi.com`
  - **改用 `go:embed`**：正式镜像是多阶段构建，源码树里的数据文件不会进运行层，依赖相对路径会导致容器内该接口永远 503。数据文件位于 `internal/service/provider_pricing_default.json`。
- [x] 3.8 `internal/service/domain_constants.go` 加 `SettingKeyReconciliationFXUSDCNYRate`
  - **改为不加**：汇率运行时覆盖落在 `reconciliation_sync_state` 的 `fx_usd_cny_rate_override` 键上，不动 `settings` 表（避免与 `api_contract_test.go` 的 `SystemSettings` 完整 JSON 比对冲突）。
- [x] 3.9 **追加** `internal/service/reconciliation_ops.go`：汇率运行时覆盖、历史回填（分段 + 游标 + 状态持久化）、手工导入上游记录
- [x] 3.10 **追加** `internal/service/reconciliation_a6_bill_source.go`：把 A6 客户端适配成 `ReconciliationUpstreamBillSource` 端口

## 4. handler 层（保 11 路由契约）

- [x] 4.1 `internal/handler/admin/companion.go`：11 个方法，**每个上方两行中文注释（说明 + `METHOD /path`）**
  - **沿用原文件名、原类型名 `CompanionHandler` 与原 11 个方法名**，这样 `routes/admin.go` 的路由注册**零 diff**，只改构造函数签名。
- [x] 4.2 错误映射：400/500/503 + **`message` 里保留 `COMPANION_*` 前缀**；**绝不返回 401/403**
- [x] 4.3 `Status` 的 `enabled` 恒 true；`collect` 立即返回不阻塞
- [x] 4.4 金额 8 位小数字符串、`margin_percent` 2 位小数且**永不为 null**；未对账的 `upstream_cost`/`gross_profit` 给 `""`
- [x] 4.5 计数字段确保是 JSON number
- [x] 4.6 `internal/handler/provider_pricing_handler.go`：公开接口，`cache_create_price` 不带 omitempty
- [x] 4.7 删除 `internal/handler/admin/companion.go` 的反代实现与 `COMPANION_*` 的 `os.Getenv`
- [x] 4.8 **追加**：`POST /a6/backfill` 用 `response.Accepted` 返回 **202**

## 5. 路由与依赖注入

- [x] 5.1 `internal/server/routes/admin.go`：`registerCompanionRoutes` 路径**一字不改**，指向新 handler（随后仅更新了 2 行已失实的注释）
- [x] 5.2 `internal/server/routes/common.go`：加 `GET /api/provider/pricing`（**在 `/api/v1` 之外，无鉴权**）
- [x] 5.3 `internal/handler/handler.go`：`AdminHandlers` 结构体字段（对账走 `Admin.Companion`，类型不变）；**追加** `Handlers.ProviderPricing`
- [x] 5.4 `internal/handler/wire.go`：ProviderSet + `ProvideAdminHandlers` 参数表末尾 + `ProvideHandlers` 参数
- [x] 5.5 `internal/repository/wire.go`：ProviderSet 注册 **6 个** repo
- [x] 5.6 `internal/service/wire.go`：ProviderSet 注册 service + `ProvideReconciliationCollector` + `ProvideProviderPricingService`（适配函数见 `reconciliation_wire.go`）
- [x] 5.7 `internal/config/config.go`：新增 `ReconciliationConfig` 段 **+ 每条都配 `viper.SetDefault`**（`TestConfigKeysAreEnvReachable` 通过）
- [x] 5.8 `cmd/server/wire.go`：`provideCleanup` 参数表 + `parallelSteps` 加 `Stop()`
- [x] 5.9 `cmd/server/wire_gen_test.go`：同步新参数
- [x] 5.10 跑 `go generate ./cmd/server` **重生成 `wire_gen.go`**（未手改）

## 6. 归档

- [x] 6.1 `extensions/sub2api-companion/` → `extensions/archive/sub2api-companion/`（`git mv`，26 个文件全部记为 `R`，历史保留）
- [x] 6.2 归档目录 README 顶部加「已收编进主程序，仅作历史参考，不再维护」
- [x] 6.3 交付文档写明：生产 SQLite 备份是**运维动作**，且历史数据**不导入主库**

## 7. 测试

- [x] 7.1 `internal/handler/admin/companion_test.go`（`//go:build unit`）：11 路由信封 + 状态码 + **契约断言（计数字段是 number、`margin_percent` 非 null、无 401/403、`enabled` 恒 true、未知 `timezone` 参数被容忍）**
- [x] 7.2 `internal/service/reconciliation_sync_service_test.go` + `reconciliation_a6_client_test.go`（unit）：覆盖**四级匹配各自命中与并列拒绝**、6 种状态标签非空、窗口与分桶、汇率冻结、规则校验
- [x] 7.3 `internal/repository/reconciliation_repo_integration_test.go`（`//go:build integration`，12 个用例）：**真库**验证 6 种状态分类、Bug 1 与 Bug 2 回归、唯一部分索引「一笔调用最多一笔账单」、幂等 upsert 与汇率冻结、Summary 口径、状态守卫
- [x] 7.4 **追加** `internal/handler/provider_pricing_handler_test.go`：锁死第三方契约（顶层键集合、`cache_create_price` 键恒在、体尾换行、embed 默认数据可用、坏覆盖回落内置）

## 8. 交付前验证链

- [x] 8.1 `gofmt -l .` 无输出
- [x] 8.2 `go build ./...` exit 0
- [x] 8.3 `go test -tags=unit ./...`
  - ⚠️ 有 **1 个既有失败**：`TestOllamaProbeCallback_StaleLongDoesNotOverrideNewShort`（位于 `ratelimit_service_ollama_429_test.go`，与对账无关）。已在**纯净 `HEAD` 检出**上复现同样失败，证实是 `dev` 分支既有问题，非本次引入。
- [x] 8.4 `go test -tags=integration ./internal/repository/`
  - ⚠️ 有 **3 个既有失败**：`TestPgDumper*`（与对账无关），同样已在纯净 `HEAD` 上复现。**本次新增的 12 个对账集成用例全绿。**
- [x] 8.5 `pnpm run check:i18n` exit 0
- [x] 8.6 `pnpm exec vue-tsc --noEmit` exit 0
- [x] 8.7 `pnpm run build` exit 0
- [x] 8.8 **前端零改动自查**：`git status --porcelain frontend/` 为空

---

## 实施偏差（与 `design.md` / 原始计划不一致处，逐条说明）

1. **表数量 5 → 4。** 早期设计稿列出 5 张表；实际 4 张即可覆盖全部功能：不再需要「请求 ID 映射表」，因为 `usage_logs.upstream_request_id`（迁移 232 + 部分索引 233 + `internal/service/upstream_request_id.go`）**主程序早已具备**，旧 Companion 那套 Caddy 日志解析要解决的问题在主程序里已经解决。少一张表、少一处写入热路径。
2. **状态数量 11 → 6。** 精确为：`billed` / `pending` / `rule_unconfigured` / `a6_pending` / `a6_waiting` / `upstream_unmatched`。其中 `pending` 表示「该调用还没有被采集到快照」，是一个真实且短暂的状态。
3. **汇率「面板可配置」的实现方式。** 前端**没有汇率输入框**（`CompanionView.vue` 只展示 `fx_usd_cny`），而前端必须保持零改动，因此汇率做成**运行时可通过两种途径调整**：配置文件 `reconciliation.fx_usd_cny_rate`，或同步状态表键 `fx_usd_cny_rate_override`（优先级更高，置 ≤0 清除）。每条记录写入时冻结当时汇率，改汇率不重算历史。
4. **`pending` 与 `rule_unconfigured` 的分界。** 只要快照行缺失就算 `pending`（采集器 30 秒一轮会补上），仅当**快照与当前规则都没有 provider** 时才判 `rule_unconfigured`。这样「账号配了规则、只是账单还没到」会正确落到 `a6_waiting`，而不是被误标成「规则待配置」。
5. **孤儿账单成本不计入汇总。** `upstream_cost` 只累计**已匹配**到下游调用的账单，保证 `gross_profit = matched_revenue − upstream_cost` 与 `profit_scope = matched_only` 自洽。未匹配账单的成本只作为明细行呈现（契约里没有能装下它的汇总字段）。
6. **A6 直连匹配的运维前提。** `usage_logs.upstream_request_id` 只有在每个 A6 账号配置了 `extra.upstream_request_id_header` 之后才有值；不配则自动降级到组合匹配（第 2~4 级），匹配率明显更低。这是**配置动作，不是代码问题**，必须写进运维文档。
7. **公开价格接口的响应形状不走通用信封。** 它是第三方已经依赖的旧 companion `{schema_version, success, message, data}` 形状，**不是**本项目的 `{code, message, data}`。这是刻意的，不要"统一"它。已用字节级对照与生产实测坐实。
8. **`api_contract_test.go` 约束。** 汇率覆盖因此不放进 `SystemSettings` 的 DTO，否则会破坏该测试的完整 JSON 比对。
