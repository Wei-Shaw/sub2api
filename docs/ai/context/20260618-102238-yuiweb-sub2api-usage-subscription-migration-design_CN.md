# yui.web 用量与套餐迁移到 Sub2API 的对齐设计

## 背景

当前已将 yui.web 的 21 个手机号用户迁移为 Sub2API 用户，临时邮箱格式为 `<手机号>@phone.com`。下一步需要判断 yui.web 的套餐档位、当前用量和历史用量能否迁移到 Sub2API。

## yui.web 当前模型

yui.web 的 shop 模型分为三类：

1. 套餐档位：`subscription_plans`
   - `sub_29_daily_19_usd`：29 元 / 30 天 / 每日 19 USD 额度
   - `sub_39_daily_29_usd`：39 元 / 30 天 / 每日 29 USD 额度
   - `sub_59_daily_49_usd`：59 元 / 30 天 / 每日 49 USD 额度
2. 用户订阅：`account_subscriptions`
   - 当前活跃订阅：12 个，全部是 `sub_29_daily_19_usd`
   - 另有 1 个 `sub_39_daily_29_usd` 已 cancelled
3. 用量与扣费：
   - `usage_events`：14722 条原始请求事件，主要用于历史展示和统计
   - `api_usd_charge_records`：按官方 USD 额度扣费的计费流水
   - 当前 charged 计费记录约 1446 条，涉及 9 个用户
   - 2026-06-18 活跃订阅用户中只有 1 个用户消耗了当日套餐额度，合计约 2.805383 USD
   - 当前加量包余额：0 个账户有非零余额

## Sub2API 当前模型

Sub2API 不是只展示用量。它有可用的真实套餐与计费模型：

1. `groups`
   - 可设置 `subscription_type='subscription'`
   - 可设置 `daily_limit_usd` / `weekly_limit_usd` / `monthly_limit_usd`
   - 当前已有 `codex-pool` 分组：platform=`openai`，daily_limit_usd=`19`
2. `subscription_plans`
   - 当前已有 `29 元订阅池`，绑定 `codex-pool`，价格 29，周期 30 天
3. `user_subscriptions`
   - 记录用户订阅、开始/过期时间、当前 daily/weekly/monthly usage
4. `usage_logs`
   - 记录 Sub2API 网关真实请求日志
   - 需要有效 `user_id`、`api_key_id`、`account_id`、可选 `group_id` / `subscription_id`
5. `billing_usage_entries`
   - 与 `usage_logs` 一对一，记录余额或订阅扣费类型

## 对齐结论

可以直接对应：

- yui.web 的 `29 元 / 每日 19 USD / 30 天` 可以对应 Sub2API 的 `codex-pool` + `29 元订阅池`。
- yui.web 的 `39 元 / 每日 29 USD / 30 天` 和 `59 元 / 每日 49 USD / 30 天` 可以在 Sub2API 新建对应 `groups` 和 `subscription_plans`。
- yui.web 的活跃订阅可以迁移到 Sub2API `user_subscriptions`。
- yui.web 的当日已用套餐额度可以迁移到 `user_subscriptions.daily_usage_usd`，保证用户当天剩余额度正确。

不能直接一比一迁移：

- yui.web 的 `usage_events` 是旧系统请求事件，Sub2API 的 `usage_logs` 是网关真实请求日志，带有强外键：`api_key_id`、`account_id`。直接导入需要创建合成 API Key / 合成账号，容易污染 Sub2API 后续运营统计。
- yui.web 的长期加量包余额没有 Sub2API 原生等价表。当前加量包余额为 0，因此这次可以不迁移。
- yui.web 的人民币余额/授信和 Sub2API 用户 `balance` 都叫余额，但语义不同。yui.web 当前余额总额为负，且属于旧 shop 账本，不建议直接变成 Sub2API 用户余额。

## 推荐迁移方案

第一阶段只迁移会影响用户继续使用的事实源：

1. 补齐 Sub2API 的 39/59 两个套餐档位：
   - 新建 `groups`：daily_limit_usd 分别为 29、49
   - 新建 `subscription_plans`：价格分别为 39、59，周期 30 天
2. 迁移 yui.web 当前 12 个 active 订阅到 Sub2API `user_subscriptions`：
   - `starts_at` / `expires_at` 使用 yui.web 原值
   - `group_id` 按 plan 映射
   - `status='active'`
   - `daily_window_start` 使用 Sub2API 当前时区当天 00:00（Asia/Shanghai）
   - `daily_usage_usd` 使用 yui.web 2026-06-18 当日 active 订阅用户已扣套餐额度
   - `weekly_usage_usd` / `monthly_usage_usd` 可使用从 2026-06-17 起本周期内同用户 `daily_quota_deducted_usd_micros` 聚合值
3. 清理受影响用户的 Redis 订阅计费缓存 `billing:sub:<user_id>:<group_id>`，让下一次请求从 PostgreSQL 回填。

第二阶段再决定是否做历史展示：

- 如果只是要用户接着用，不迁历史 `usage_logs`。
- 如果必须在 Sub2API 里展示历史趋势，建议新增一个“legacy usage import”只读归档表或单独页面，而不是硬写 `usage_logs`。

## 风险

- 直接写 `usage_logs` 会需要合成 API Key 和上游 account，后续管理员报表会混入旧系统数据。
- 如果只迁 active 订阅而不迁当日 usage，用户当天额度会被重置，可能多用。
- 如果迁移 `daily_usage_usd` 后不清 Redis 缓存，运行时可能继续读旧缓存。

