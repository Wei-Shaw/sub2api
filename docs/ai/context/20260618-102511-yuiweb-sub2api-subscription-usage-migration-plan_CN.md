# yui.web 订阅与当前用量迁移到 Sub2API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 yui.web 当前 active 订阅和当日已用套餐额度迁移到 Sub2API，让老用户可以继续按原套餐额度使用。

**Architecture:** 本阶段只迁移会影响继续使用的事实源：Sub2API `groups`、`subscription_plans`、`user_subscriptions` 和 Redis 订阅缓存。不迁移 yui.web 历史 `usage_events` 到 Sub2API `usage_logs`，避免制造合成 API Key / 合成上游账号并污染后续报表。

**Tech Stack:** SQLite (`/Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite`)、PostgreSQL container (`sub2api-postgres`)、Redis container (`sub2api-redis`)、Sub2API 本地服务 (`127.0.0.1:18080`)。

---

## Scope

本计划只执行第一阶段迁移：

- 新建或补齐 Sub2API 39 元 / 59 元订阅档位。
- 将 yui.web 当前 `account_subscriptions.status='active'` 的 12 个订阅迁到 Sub2API。
- 将 2026-06-18 当日 active 订阅用户已扣套餐额度迁到 `user_subscriptions.daily_usage_usd`。
- 写入周/月聚合用量，保证 Sub2API 控制中心展示不从 0 开始。
- 清理 Redis 订阅缓存，让运行时从 PostgreSQL 回填。

本计划不做：

- 不把 yui.web `usage_events` 导入 Sub2API `usage_logs`。
- 不迁 yui.web 人民币余额/授信到 Sub2API `users.balance`。
- 不迁加量包余额；当前 yui.web 非零加量包账户为 0。
- 不修改源码文件。

## Files And Data Sources

**Read:**

- `/Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite`
- Sub2API PostgreSQL：`users`、`groups`、`subscription_plans`、`user_subscriptions`
- Sub2API Redis：`billing:sub:<user_id>:<group_id>`

**Create:**

- PostgreSQL 备份：`/Users/wujianxiang/CodeSpace/sub2api/.tmp-sub2api-before-subscription-usage-migration-${STAMP}.dump`
- SQLite 备份：`/Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-shop-before-subscription-usage-migration-${STAMP}.sqlite`
- 迁移结果文档：`/Users/wujianxiang/CodeSpace/sub2api/docs/ai/context/${STAMP}-yuiweb-sub2api-subscription-usage-migration-result_CN.md`

**Modify:**

- PostgreSQL `groups`
- PostgreSQL `subscription_plans`
- PostgreSQL `user_subscriptions`
- Redis `billing:sub:*` 相关缓存 key

## Plan Tasks

### Task 1: Preflight Dry Run

**Files:**

- Read: `/Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite`
- Read: PostgreSQL `users`, `groups`, `subscription_plans`, `user_subscriptions`

- [ ] **Step 1: Verify yui.web plan inventory**

Run:

```bash
sqlite3 -header -column /Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite \
  "select id, name, monthly_price_cents, printf('%.6f', daily_quota_usd_micros / 1000000.0) as daily_quota_usd, period_days, status from subscription_plans order by monthly_price_cents;"
```

Expected:

```text
sub_29_daily_19_usd | 29 元订阅池 | 2900 | 19.000000 | 30 | active
sub_39_daily_29_usd | 39 元订阅池 | 3900 | 29.000000 | 30 | active
sub_59_daily_49_usd | 59 元订阅池 | 5900 | 49.000000 | 30 | active
```

- [ ] **Step 2: Verify active subscription count**

Run:

```bash
sqlite3 -header -column /Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite \
  "select sp.id as plan_id, s.status, count(*) as users from account_subscriptions s join subscription_plans sp on sp.id=s.plan_id group by sp.id, s.status order by sp.id, s.status;"
```

Expected:

```text
sub_29_daily_19_usd | active | 12
sub_39_daily_29_usd | cancelled | 1
```

- [ ] **Step 3: Verify every active yui.web subscription has a migrated Sub2API user**

Run:

```bash
sqlite3 -header -column /Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite \
  "select count(*) as active_subscription_users from account_subscriptions where status='active';"

PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-postgres \
  psql -U sub2api -d sub2api -t -A -c \
  "select count(*) from users where email like '%@phone.com' and deleted_at is null;"
```

Expected:

```text
active_subscription_users = 12
Sub2API @phone.com users >= 12
```

- [ ] **Step 4: Verify current Sub2API subscription state**

Run:

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-postgres \
  psql -U sub2api -d sub2api -t -A -F $'\t' -c \
  "select id, name, platform, subscription_type, daily_limit_usd, default_validity_days, status from groups where deleted_at is null order by id;
   select id, group_id, name, price, validity_days, for_sale from subscription_plans order by id;
   select count(*) from user_subscriptions where deleted_at is null;"
```

Expected:

```text
groups contains codex-pool with daily_limit_usd = 19
subscription_plans contains 29 元订阅池
user_subscriptions count is known before migration
```

### Task 2: Back Up Databases

**Files:**

- Create: `/Users/wujianxiang/CodeSpace/sub2api/.tmp-sub2api-before-subscription-usage-migration-${STAMP}.dump`
- Create: `/Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-shop-before-subscription-usage-migration-${STAMP}.sqlite`

- [ ] **Step 1: Back up Sub2API PostgreSQL**

Run with a fresh timestamp:

```bash
STAMP=$(date +%Y%m%d-%H%M%S)
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-postgres \
  pg_dump -U sub2api -d sub2api --format=custom \
  --file=/tmp/sub2api-before-subscription-usage-migration-${STAMP}.dump

PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker cp \
  sub2api-postgres:/tmp/sub2api-before-subscription-usage-migration-${STAMP}.dump \
  /Users/wujianxiang/CodeSpace/sub2api/.tmp-sub2api-before-subscription-usage-migration-${STAMP}.dump

ls -lh /Users/wujianxiang/CodeSpace/sub2api/.tmp-sub2api-before-subscription-usage-migration-${STAMP}.dump
```

Expected:

```text
dump file exists and size is non-zero
```

- [ ] **Step 2: Back up yui.web SQLite**

Run:

```bash
STAMP=$(date +%Y%m%d-%H%M%S)
cp /Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite \
  /Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-shop-before-subscription-usage-migration-${STAMP}.sqlite

ls -lh /Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-shop-before-subscription-usage-migration-${STAMP}.sqlite
```

Expected:

```text
sqlite backup exists and size is non-zero
```

### Task 3: Build Migration Staging Data

**Files:**

- Create temporary CSV: `/Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-active-subscriptions-for-sub2api.csv`
- Read: `/Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite`

- [ ] **Step 1: Generate active subscription staging CSV**

Run:

```bash
sqlite3 -csv /Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite "
select
  lower(trim(s.phone)) || '@phone.com' as email,
  s.plan_id,
  s.started_at,
  s.expires_at,
  coalesce((
    select sum(c.daily_quota_deducted_usd_micros) / 1000000.0
    from api_usd_charge_records c
    where c.phone = s.phone
      and c.status = 'charged'
      and c.quota_date = '2026-06-18'
  ), 0.0) as daily_usage_usd,
  coalesce((
    select sum(c.daily_quota_deducted_usd_micros) / 1000000.0
    from api_usd_charge_records c
    where c.phone = s.phone
      and c.status = 'charged'
      and c.created_at >= s.started_at
      and c.created_at < s.expires_at
  ), 0.0) as period_usage_usd
from account_subscriptions s
where s.status = 'active'
order by s.phone;
" > /Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-active-subscriptions-for-sub2api.csv

wc -l /Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-active-subscriptions-for-sub2api.csv
```

Expected:

```text
12 /Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-active-subscriptions-for-sub2api.csv
```

- [ ] **Step 2: Copy staging CSV into PostgreSQL container**

Run:

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker cp \
  /Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-active-subscriptions-for-sub2api.csv \
  sub2api-postgres:/tmp/yuiweb-active-subscriptions-for-sub2api.csv
```

Expected:

```text
command exits 0
```

### Task 4: Upsert Sub2API Plan Catalog

**Files:**

- Modify: PostgreSQL `groups`
- Modify: PostgreSQL `subscription_plans`

- [ ] **Step 1: Execute group and plan upsert transaction**

Run:

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec -i sub2api-postgres \
  psql -U sub2api -d sub2api <<'SQL'
\set ON_ERROR_STOP on
BEGIN;

UPDATE groups
SET
  platform = 'openai',
  subscription_type = 'subscription',
  daily_limit_usd = 19,
  weekly_limit_usd = NULL,
  monthly_limit_usd = NULL,
  default_validity_days = 30,
  status = 'active',
  updated_at = now()
WHERE name = 'codex-pool'
  AND deleted_at IS NULL;

INSERT INTO groups (
  name,
  description,
  rate_multiplier,
  is_exclusive,
  status,
  platform,
  subscription_type,
  daily_limit_usd,
  weekly_limit_usd,
  monthly_limit_usd,
  default_validity_days,
  sort_order,
  created_at,
  updated_at
)
SELECT
  v.name,
  v.description,
  1.0,
  false,
  'active',
  'openai',
  'subscription',
  v.daily_limit_usd,
  NULL,
  NULL,
  30,
  v.sort_order,
  now(),
  now()
FROM (
  VALUES
    ('codex-pool-29-usd', 'yui.web 39 元订阅池迁移：每日 29 USD', 29::numeric, 39),
    ('codex-pool-49-usd', 'yui.web 59 元订阅池迁移：每日 49 USD', 49::numeric, 59)
) AS v(name, description, daily_limit_usd, sort_order)
WHERE NOT EXISTS (
  SELECT 1 FROM groups g WHERE g.name = v.name AND g.deleted_at IS NULL
);

INSERT INTO subscription_plans (
  group_id,
  name,
  description,
  price,
  original_price,
  validity_days,
  validity_unit,
  features,
  product_name,
  for_sale,
  sort_order,
  created_at,
  updated_at
)
SELECT
  g.id,
  v.plan_name,
  v.description,
  v.price,
  NULL,
  30,
  'day',
  v.features,
  v.plan_name,
  true,
  v.sort_order,
  now(),
  now()
FROM (
  VALUES
    ('codex-pool', '29 元订阅池', 'yui.web 29 元订阅池迁移：每日 19 USD', 29::numeric, '每日 19 USD 额度，有效期 30 天', 29),
    ('codex-pool-29-usd', '39 元订阅池', 'yui.web 39 元订阅池迁移：每日 29 USD', 39::numeric, '每日 29 USD 额度，有效期 30 天', 39),
    ('codex-pool-49-usd', '59 元订阅池', 'yui.web 59 元订阅池迁移：每日 49 USD', 59::numeric, '每日 49 USD 额度，有效期 30 天', 59)
) AS v(group_name, plan_name, description, price, features, sort_order)
JOIN groups g ON g.name = v.group_name AND g.deleted_at IS NULL
WHERE NOT EXISTS (
  SELECT 1
  FROM subscription_plans sp
  WHERE sp.name = v.plan_name
    AND sp.group_id = g.id
);

COMMIT;
SQL
```

Expected:

```text
COMMIT
```

- [ ] **Step 2: Verify plan catalog**

Run:

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-postgres \
  psql -U sub2api -d sub2api -t -A -F $'\t' -c \
  "select name, platform, subscription_type, daily_limit_usd, default_validity_days, status from groups where name in ('codex-pool','codex-pool-29-usd','codex-pool-49-usd') and deleted_at is null order by daily_limit_usd;
   select sp.name, g.name, sp.price, sp.validity_days, sp.for_sale from subscription_plans sp join groups g on g.id=sp.group_id where sp.name in ('29 元订阅池','39 元订阅池','59 元订阅池') order by sp.price;"
```

Expected:

```text
codex-pool       | openai | subscription | 19 | 30 | active
codex-pool-29-usd | openai | subscription | 29 | 30 | active
codex-pool-49-usd | openai | subscription | 49 | 30 | active
29 元订阅池 | codex-pool        | 29 | 30 | t
39 元订阅池 | codex-pool-29-usd | 39 | 30 | t
59 元订阅池 | codex-pool-49-usd | 59 | 30 | t
```

### Task 5: Migrate Active User Subscriptions

**Files:**

- Modify: PostgreSQL `user_subscriptions`

- [ ] **Step 1: Execute active subscription migration transaction**

Run:

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec -i sub2api-postgres \
  psql -U sub2api -d sub2api <<'SQL'
\set ON_ERROR_STOP on
BEGIN;

CREATE TEMP TABLE tmp_yuiweb_active_subscriptions (
  email text NOT NULL,
  yui_plan_id text NOT NULL,
  started_at_text text NOT NULL,
  expires_at_text text NOT NULL,
  daily_usage_usd numeric(20,10) NOT NULL,
  period_usage_usd numeric(20,10) NOT NULL
);

\copy tmp_yuiweb_active_subscriptions(email, yui_plan_id, started_at_text, expires_at_text, daily_usage_usd, period_usage_usd) FROM '/tmp/yuiweb-active-subscriptions-for-sub2api.csv' WITH (FORMAT csv)

WITH mapped AS (
  SELECT
    u.id AS user_id,
    CASE t.yui_plan_id
      WHEN 'sub_29_daily_19_usd' THEN g19.id
      WHEN 'sub_39_daily_29_usd' THEN g29.id
      WHEN 'sub_59_daily_49_usd' THEN g49.id
    END AS group_id,
    t.yui_plan_id,
    t.started_at_text::timestamptz AS starts_at,
    t.expires_at_text::timestamptz AS expires_at,
    t.daily_usage_usd,
    t.period_usage_usd
  FROM tmp_yuiweb_active_subscriptions t
  JOIN users u ON u.email = t.email AND u.deleted_at IS NULL
  LEFT JOIN groups g19 ON g19.name = 'codex-pool' AND g19.deleted_at IS NULL
  LEFT JOIN groups g29 ON g29.name = 'codex-pool-29-usd' AND g29.deleted_at IS NULL
  LEFT JOIN groups g49 ON g49.name = 'codex-pool-49-usd' AND g49.deleted_at IS NULL
)
INSERT INTO user_subscriptions (
  user_id,
  group_id,
  starts_at,
  expires_at,
  status,
  daily_window_start,
  weekly_window_start,
  monthly_window_start,
  daily_usage_usd,
  weekly_usage_usd,
  monthly_usage_usd,
  assigned_by,
  assigned_at,
  notes,
  created_at,
  updated_at
)
SELECT
  user_id,
  group_id,
  starts_at,
  expires_at,
  'active',
  date_trunc('day', now()),
  date_trunc('week', now()),
  starts_at,
  daily_usage_usd,
  period_usage_usd,
  period_usage_usd,
  NULL,
  now(),
  'migrated_from_yuiweb_active_subscription_20260618 plan=' || yui_plan_id,
  now(),
  now()
FROM mapped
WHERE group_id IS NOT NULL
ON CONFLICT (user_id, group_id) WHERE deleted_at IS NULL
DO UPDATE SET
  starts_at = EXCLUDED.starts_at,
  expires_at = EXCLUDED.expires_at,
  status = 'active',
  daily_window_start = EXCLUDED.daily_window_start,
  weekly_window_start = EXCLUDED.weekly_window_start,
  monthly_window_start = EXCLUDED.monthly_window_start,
  daily_usage_usd = EXCLUDED.daily_usage_usd,
  weekly_usage_usd = EXCLUDED.weekly_usage_usd,
  monthly_usage_usd = EXCLUDED.monthly_usage_usd,
  notes = EXCLUDED.notes,
  updated_at = now();

COMMIT;
SQL
```

Expected:

```text
COMMIT
```

- [ ] **Step 2: Verify migrated subscription count**

Run:

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-postgres \
  psql -U sub2api -d sub2api -t -A -F $'\t' -c \
  "select g.name, count(*) as active_users, sum(us.daily_usage_usd) as daily_usage_usd, sum(us.weekly_usage_usd) as weekly_usage_usd
   from user_subscriptions us
   join groups g on g.id=us.group_id
   where us.deleted_at is null
     and us.status='active'
     and us.notes like 'migrated_from_yuiweb_active_subscription_20260618%'
   group by g.name
   order by g.name;"
```

Expected:

```text
codex-pool | 12 | approximately 2.805383 | non-negative period usage
```

- [ ] **Step 3: Verify no active yui.web subscription was skipped**

Run:

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-postgres \
  psql -U sub2api -d sub2api -t -A -c \
  "select count(*)
   from user_subscriptions us
   where us.deleted_at is null
     and us.status='active'
     and us.notes like 'migrated_from_yuiweb_active_subscription_20260618%';"
```

Expected:

```text
12
```

### Task 6: Invalidate Subscription Billing Cache

**Files:**

- Modify: Redis keys `billing:sub:<user_id>:<group_id>`

- [ ] **Step 1: Export Redis keys to delete**

Run:

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-postgres \
  psql -U sub2api -d sub2api -t -A -F ':' -c \
  "select us.user_id, us.group_id
   from user_subscriptions us
   where us.deleted_at is null
     and us.status='active'
     and us.notes like 'migrated_from_yuiweb_active_subscription_20260618%';" \
  > /Users/wujianxiang/CodeSpace/sub2api/.tmp-subscription-cache-keys-20260618.txt
```

Expected:

```text
file contains 12 lines
```

- [ ] **Step 2: Delete corresponding Redis keys**

Run:

```bash
while IFS=: read -r user_id group_id; do
  [ -n "$user_id" ] || continue
  PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-redis \
    redis-cli DEL "billing:sub:${user_id}:${group_id}"
done < /Users/wujianxiang/CodeSpace/sub2api/.tmp-subscription-cache-keys-20260618.txt
```

Expected:

```text
Each command exits 0. Return values may be 0 or 1 depending on whether the key existed.
```

- [ ] **Step 3: Remove temporary files**

Run:

```bash
rm -f /Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-active-subscriptions-for-sub2api.csv
rm -f /Users/wujianxiang/CodeSpace/sub2api/.tmp-subscription-cache-keys-20260618.txt
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-postgres \
  rm -f /tmp/yuiweb-active-subscriptions-for-sub2api.csv
```

Expected:

```text
temporary staging files removed
```

### Task 7: Runtime Verification

**Files:**

- Read: Sub2API API at `http://127.0.0.1:18080`
- Read: PostgreSQL `user_subscriptions`

- [ ] **Step 1: Verify one migrated user has active subscription in database**

Run:

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-postgres \
  psql -U sub2api -d sub2api -t -A -F $'\t' -c \
  "select left(u.email, 2) || '***@phone.com' as masked_email,
          g.name,
          us.status,
          us.daily_usage_usd,
          g.daily_limit_usd,
          us.expires_at
   from user_subscriptions us
   join users u on u.id=us.user_id
   join groups g on g.id=us.group_id
   where us.deleted_at is null
     and us.status='active'
     and us.notes like 'migrated_from_yuiweb_active_subscription_20260618%'
   order by us.daily_usage_usd desc
   limit 3;"
```

Expected:

```text
Rows show masked @phone.com users, codex-pool, active, daily_usage_usd <= daily_limit_usd.
```

- [ ] **Step 2: Verify user dashboard subscription API**

Use an existing migrated login session in the browser or call the API with a freshly logged-in migrated account. If using API:

```bash
TEST_EMAIL=$(PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-postgres \
  psql -U sub2api -d sub2api -t -A -c \
  "select u.email
   from users u
   join user_subscriptions us on us.user_id=u.id
   where u.email like '%@phone.com'
     and u.deleted_at is null
     and us.deleted_at is null
     and us.status='active'
   order by us.daily_usage_usd desc
   limit 1;")

ACCESS_TOKEN=$(curl -s -X POST http://127.0.0.1:18080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  --data "{\"email\":\"${TEST_EMAIL}\",\"password\":\"123123\"}" \
  | jq -r '.data.access_token // .access_token')

curl -s http://127.0.0.1:18080/api/v1/subscriptions/active \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  | jq '{code, has_data: (.data != null), group_name: (.data.group_name // .data.group?.name // null), daily_usage_usd: (.data.daily_usage_usd // null), daily_limit_usd: (.data.daily_limit_usd // .data.group?.daily_limit_usd // null)}'
```

Expected:

```json
{
  "code": 0,
  "has_data": true
}
```

- [ ] **Step 3: Verify browser UI**

Open:

```text
http://127.0.0.1:18080/subscriptions
```

Expected:

```text
The logged-in migrated user sees an active subscription and daily quota/usage information.
```

### Task 8: Result Documentation

**Files:**

- Create: `/Users/wujianxiang/CodeSpace/sub2api/docs/ai/context/${STAMP}-yuiweb-sub2api-subscription-usage-migration-result_CN.md`

- [ ] **Step 1: Write result document**

Create a result document containing:

```markdown
# yui.web 订阅与当前用量迁移到 Sub2API 结果

## 结果

- 已补齐 Sub2API 订阅档位：29 元、39 元、59 元。
- 已迁移 yui.web active 订阅数量：12。
- 已迁移当日套餐用量聚合值：约 2.805383 USD。
- 未迁移历史 usage_events 到 Sub2API usage_logs。
- 已清理对应 Redis 订阅计费缓存。

## 备份

- Sub2API PostgreSQL 备份：`/Users/wujianxiang/CodeSpace/sub2api/.tmp-sub2api-before-subscription-usage-migration-${STAMP}.dump`
- yui.web SQLite 备份：`/Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-shop-before-subscription-usage-migration-${STAMP}.sqlite`

## 验证

- 数据库订阅数量校验通过。
- active subscription API 校验通过。
- 浏览器 `/subscriptions` 页面校验通过。

## 后续

- 后续真实请求将由 Sub2API 自己记录到 `usage_logs`。
- 若需要展示 yui.web 历史趋势，单独设计 legacy usage 只读归档，不污染 Sub2API 原生 usage_logs。
```

Expected:

```text
Result document exists under docs/ai/context/.
```

## Rollback Plan

如果迁移后需要回滚：

1. 恢复 PostgreSQL：

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker cp \
  /Users/wujianxiang/CodeSpace/sub2api/.tmp-sub2api-before-subscription-usage-migration-${STAMP}.dump \
  sub2api-postgres:/tmp/rollback-subscription-usage.dump

PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec -i sub2api-postgres \
  pg_restore -U sub2api -d sub2api --clean --if-exists /tmp/rollback-subscription-usage.dump
```

2. 清 Redis 订阅缓存：

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-redis \
  redis-cli --scan --pattern 'billing:sub:*' | \
  xargs -r PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker exec sub2api-redis redis-cli DEL
```

3. 重启 Sub2API 容器：

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker restart sub2api
```

## Self Review

- 覆盖了设计文档中的第一阶段范围：套餐档位、active 订阅、当日用量、Redis 缓存、验证与结果文档。
- 未包含历史 `usage_events -> usage_logs` 导入，符合设计中的风险约束。
- 所有 SQL 使用明确字段名；没有依赖完整手机号输出。
- 临时文件在执行后会删除，备份文件保留。
