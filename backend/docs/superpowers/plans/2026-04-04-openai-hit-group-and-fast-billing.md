# OpenAI Hit-Group And Fast Billing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 OpenAI 请求在写 usage 时固化“优先账号倍率 + 实际单价 + 价格来源”，并让余额计费、订阅累计、用户侧明细与统计统一按新 `actual_cost` 口径工作。

**Architecture:** 实现分三层推进：先修 usage 写入规则，把 `priority` 只当作单价来源而不是额外 `2x` 倍率；再把新字段落库并回填旧数据；最后让用户侧 UI 读新口径并展示完整倍率因子、实际单价和价格来源，同时继续隐藏 exhausted/non-exhausted 内部概念。

**Tech Stack:** Go, PostgreSQL/ent migrations, existing billing/usage services, Vue 3, pnpm, user usage page.

---

## 文件边界

- Modify: `backend/internal/service/openai_gateway_service.go`
  OpenAI usage 写库主链路，新增优先账号倍率、最终倍率、实际单价与价格来源计算。
- Modify: `backend/internal/service/gateway_service.go`
  通用余额/订阅累计链路，确保两条链路都统一使用新 `actual_cost`。
- Modify: `backend/internal/service/usage_log.go`
  扩展 usage 领域模型，补齐优先账号倍率、最终倍率、实际单价、价格来源字段。
- Modify: `backend/internal/repository/usage_log_repo.go`
  usage 写库、扫描、统计查询统一接新字段与新 `actual_cost` 口径。
- Modify: `backend/internal/repository/usage_billing_repo.go`
  余额/订阅累计写入逻辑统一读新 `actual_cost`。
- Modify: `backend/internal/repository/user_subscription_repo.go`
  如订阅累计查询/更新仍使用旧口径，则切换到新 `actual_cost`。
- Modify: `backend/ent/schema/usage_log.go`
  为新增字段补 schema。
- Create: `backend/migrations/<next>_add_openai_billing_breakdown.sql`
  新增字段并提供历史 usage / subscription 回填 SQL。
- Modify: `backend/internal/service/openai_gateway_record_usage_test.go`
  锁定写 usage 时的新计费规则。
- Modify/Create: `backend/internal/repository/usage_log_repo_request_type_test.go`
  锁定 repo 层新字段写入/读取与统计口径。
- Modify/Create: `backend/internal/service/gateway_service_test.go`
  锁定余额/订阅两条链路都使用新 `actual_cost`。
- Modify: `frontend/src/views/user/UsageView.vue`
  用户侧明细/统计卡片展示新口径、实际单价与价格来源。
- Modify: `frontend/src/types/index.ts`
  补前端需要的新 billing breakdown 字段。
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
  增加“优先账号 100x”“Fast/priority 单价生效”和 `-Sys` 引导文案。
- Create/Modify: `frontend/src/views/user/__tests__/usage_billing_breakdown.spec.ts`
  锁定用户侧展示行为。

## Task 1: 修正 usage 写入规则，去掉 Fast 额外 2x，补齐单价与价格来源

**Files:**
- Modify: `backend/internal/service/openai_gateway_record_usage_test.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Modify: `backend/internal/service/usage_log.go`

- [ ] **Step 1: 先写失败测试，锁定修正版业务规则**

在 `backend/internal/service/openai_gateway_record_usage_test.go` 至少新增以下测试：

```go
func TestOpenAIGatewayServiceRecordUsage_ActiveTargetGroupAppliesPriorityAccountMultiplierOnly(t *testing.T) {}
func TestOpenAIGatewayServiceRecordUsage_PriorityServiceTierUsesPriorityUnitPriceWithoutExtraMultiplier(t *testing.T) {}
func TestOpenAIGatewayServiceRecordUsage_StoresEffectiveUnitPricesAndPricingSource(t *testing.T) {}
func TestOpenAIGatewayServiceRecordUsage_ActiveTargetGroupAndPriorityTierCombineWithoutFastExtraMultiplier(t *testing.T) {}
```

每个测试都要构造：
- `RoutingSnapshot.TargetGroup`
- `Result.ServiceTier`
- 已存在 `rate_multiplier`
- `account_rate_multiplier`

并断言 usageLog 最终写出的：
- `PriorityAccountMultiplier`
- `EffectiveMultiplier`
- `EffectiveInputUnitPrice`
- `EffectiveOutputUnitPrice`
- `EffectiveCacheReadUnitPrice`（如适用）
- 价格来源标记
- `ActualCost`

- [ ] **Step 2: 跑测试确认 RED**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -run 'TestOpenAIGatewayServiceRecordUsage_(ActiveTargetGroupAppliesPriorityAccountMultiplierOnly|PriorityServiceTierUsesPriorityUnitPriceWithoutExtraMultiplier|StoresEffectiveUnitPricesAndPricingSource|ActiveTargetGroupAndPriorityTierCombineWithoutFastExtraMultiplier)$' -count=1
```

Expected: FAIL，原因是当前实现还保留旧的 Fast 额外倍率思路，且还没有单价/价格来源字段。

- [ ] **Step 3: 写最小实现**

在 `backend/internal/service/usage_log.go` 增加字段，例如：

```go
PriorityAccountMultiplier *float64
EffectiveMultiplier       *float64
EffectiveInputUnitPrice   *float64
EffectiveOutputUnitPrice  *float64
EffectiveCacheReadUnitPrice *float64
PricingSource             *string
```

在 `backend/internal/service/openai_gateway_service.go` 的 `RecordUsage(...)` 中：

```go
priorityAccountMultiplier := 1.0
if snapshot != nil && strings.TrimSpace(snapshot.TargetGroup) == string(TargetGroupActive) {
    priorityAccountMultiplier = 100.0
}

effectiveMultiplier := multiplier * accountRateMultiplier * priorityAccountMultiplier
cost.ActualCost = cost.TotalCost * effectiveMultiplier
```

同时：
- 不再引入 `fastModeMultiplier = 2`
- 记录当前实际使用的输入/输出/缓存读单价
- 记录价格来源（例如 `priority_pricing`, `priority_account_multiplier` 之类的结构化标记）

- [ ] **Step 4: 跑测试确认 GREEN**

Run 同 Step 2。

Expected: PASS。

## Task 2: 持久化新字段并统一 usage / subscription / stats 口径

**Files:**
- Modify: `backend/ent/schema/usage_log.go`
- Create: `backend/migrations/<next>_add_openai_billing_breakdown.sql`
- Modify: `backend/internal/repository/usage_log_repo.go`
- Modify: `backend/internal/repository/usage_log_repo_request_type_test.go`
- Modify: `backend/internal/repository/usage_billing_repo.go`
- Modify: `backend/internal/repository/user_subscription_repo.go`

- [ ] **Step 1: 先写失败测试，锁定 repo 层新字段与统计口径**

在 `backend/internal/repository/usage_log_repo_request_type_test.go` 增加/调整测试：

```go
func TestPrepareUsageLogInsert_IncludesPriorityMultiplierAndUnitPrices(t *testing.T) {}
func TestScanUsageLog_PreservesBillingBreakdownFields(t *testing.T) {}
func TestGetUserStatsAggregated_UsesActualCostAfterPriorityBilling(t *testing.T) {}
```

在订阅累计相关 repo test 中增加：

```go
func TestSubscriptionUsageAggregation_UsesActualCostAfterPriorityBilling(t *testing.T) {}
```

- [ ] **Step 2: 跑测试确认 RED**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/repository -run 'Test(PrepareUsageLogInsert_IncludesPriorityMultiplierAndUnitPrices|ScanUsageLog_PreservesBillingBreakdownFields|GetUserStatsAggregated_UsesActualCostAfterPriorityBilling|SubscriptionUsageAggregation_UsesActualCostAfterPriorityBilling)$' -count=1
```

Expected: FAIL，原因是 schema / insert / scan / aggregate 还没有这些字段与新口径。

- [ ] **Step 3: 写最小实现**

在 `backend/ent/schema/usage_log.go` 增加字段，例如：

```go
field.Float("priority_account_multiplier").Optional().Nillable()
field.Float("effective_multiplier").Optional().Nillable()
field.Float("effective_input_unit_price").Optional().Nillable()
field.Float("effective_output_unit_price").Optional().Nillable()
field.Float("effective_cache_read_unit_price").Optional().Nillable()
field.String("pricing_source").Optional().Nillable()
```

在 migration 中新增对应列，并提供回填思路：

```sql
ALTER TABLE usage_logs ADD COLUMN ...;
UPDATE usage_logs
SET priority_account_multiplier = CASE WHEN routing_target_group = 'active' THEN 100 ELSE 1 END,
    effective_multiplier = COALESCE(rate_multiplier,1) * COALESCE(account_rate_multiplier,1) * ...,
    actual_cost = total_cost * (...),
    pricing_source = ...;
```

并统一让：
- usage insert args
- scanUsageLog
- 用户/管理员统计聚合
- 订阅累计/回填逻辑

全部使用新 `actual_cost` 与新字段。

- [ ] **Step 4: 跑测试确认 GREEN**

Run 同 Step 2。

Expected: PASS。

## Task 3: 让用户侧明细与统计展示“实际单价 + 价格来源 + 倍率因子”

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/views/user/UsageView.vue`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Create/Modify: `frontend/src/views/user/__tests__/usage_billing_breakdown.spec.ts`

- [ ] **Step 1: 先写失败测试，锁定用户侧展示结果**

新增前端测试，要求：

```ts
it('shows effective unit prices and pricing source in user usage view')
it('shows priority account multiplier and final multiplier')
it('shows actual billed totals in stats cards')
it('does not expose exhausted wording and still recommends -Sys')
```

至少断言：
- `priority_account_multiplier`
- `effective_multiplier`
- `effective_input_unit_price`
- `effective_output_unit_price`
- `pricing_source`
- `优先账号 100x`
- `Fast/priority 单价生效`
- `-Sys`

- [ ] **Step 2: 跑测试确认 RED**

Run:

```powershell
pnpm test:run "src/views/user/__tests__/usage_billing_breakdown.spec.ts"
```

Expected: FAIL，原因是用户页还没有这些字段与文案。

- [ ] **Step 3: 写最小实现**

在 `frontend/src/types/index.ts` 给用户 usage 类型补字段：

```ts
priority_account_multiplier?: number | null
effective_multiplier?: number | null
effective_input_unit_price?: number | null
effective_output_unit_price?: number | null
effective_cache_read_unit_price?: number | null
pricing_source?: string | null
```

在 `frontend/src/views/user/UsageView.vue`：
- 明细表增加实际单价、价格来源、倍率拆解展示
- 统计卡片统一显示新 `actual_cost`
- 增加“优先账号 100x”“Fast/priority 单价生效”与 `-Sys` 引导文案

在中英文 locale 中新增对应文本，避免出现 `active/exhausted` 等内部词。

- [ ] **Step 4: 跑测试确认 GREEN**

Run 同 Step 2。

Expected: PASS。

## Task 4: 全量验证并做真实请求对账

**Files:**
- Modify: none（验证阶段，除非发现缺口）

- [ ] **Step 1: 跑后端回归**

Run:

```powershell
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/service -count=1
& 'C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe' test ./internal/repository -count=1
git diff --check
```

Expected: 全部 PASS。

- [ ] **Step 2: 跑前端验证**

Run:

```powershell
pnpm test:run "src/views/user/__tests__/usage_billing_breakdown.spec.ts"
pnpm build
```

Expected: PASS。

- [ ] **Step 3: 做 4 类真实请求验算**

至少覆盖：

```text
普通请求
命中优先账号请求
priority/Fast 请求
优先账号 + Fast 同时命中的请求
```

并核对：
- `usage_logs` 的新倍率/单价字段
- `actual_cost`
- 用户侧统计接口
- 订阅累计接口

Expected: 三者一致。

- [ ] **Step 4: 输出最终交付说明**

最终说明必须明确包含：
- 最终倍率公式
- 新增落库字段
- 用户侧新增可见信息
- 是否完成历史数据回填
- 是否已同步 VPS

## 自查清单

- Spec coverage:
  - 写入时固化优先账号倍率与单价来源：Task 1
  - 新字段落库与回填：Task 2
  - 用户侧展示单价/来源/倍率：Task 3
  - 线上四类请求验算：Task 4
- Placeholder scan: 无 TBD/TODO/implement later。
- Type consistency:
  - 统一使用 `priority_account_multiplier` / `effective_multiplier`
  - 不再使用 `fast_mode_multiplier = 2` 作为正式规则
  - 用户侧文案统一用“优先账号 100x”“Fast/priority 单价生效”
