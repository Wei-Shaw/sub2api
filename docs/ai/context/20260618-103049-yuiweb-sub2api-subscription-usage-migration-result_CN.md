# yui.web 订阅与当前用量迁移到 Sub2API 结果

## 结果

- 已补齐 Sub2API 订阅档位：
  - `29 元订阅池`：`codex-pool`，每日 19 USD，30 天
  - `39 元订阅池`：`codex-pool-29-usd`，每日 29 USD，30 天
  - `59 元订阅池`：`codex-pool-49-usd`，每日 49 USD，30 天
- 已迁移 yui.web active 订阅数量：12。
- 已迁移当日套餐用量聚合值：2.805383 USD。
- 已迁移当前周期聚合用量：50.189679 USD。
- 未迁移历史 `usage_events` 到 Sub2API `usage_logs`。
- 未迁移 yui.web 人民币余额/授信到 Sub2API `users.balance`。
- 已清理对应 Redis 订阅计费缓存；目标 key 原本不存在，删除返回 0。

## 备份

- Sub2API PostgreSQL 备份：
  - `/Users/wujianxiang/CodeSpace/sub2api/.tmp-sub2api-before-subscription-usage-migration-20260618-103049.dump`
- yui.web SQLite 备份：
  - `/Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-shop-before-subscription-usage-migration-20260618-103049.sqlite`

## 验证

- yui.web dry-run：
  - 3 个套餐档位存在：29 / 39 / 59。
  - active 订阅 12 个，全部来自 `sub_29_daily_19_usd`。
- Sub2API 数据库：
  - 3 个订阅分组存在，均为 `platform=openai`、`subscription_type=subscription`。
  - 3 个订阅计划存在，价格分别为 29、39、59。
  - 迁移订阅数量为 12。
  - 迁移订阅当日用量合计为 2.805383 USD。
  - 没有迁移订阅超过每日限额。
- API：
  - 使用迁移用户默认密码登录成功。
  - `/api/v1/subscriptions/active` 返回 active 订阅数组。
- 浏览器：
  - `/subscriptions` 页面可打开。
  - 当前登录迁移用户可看到 `codex-pool` active 订阅、到期时间和每日额度。

## 注意

- 清理 Redis 缓存时 `redis-cli` 输出过 `AUTH failed` 提示，原因是容器环境中存在 redis-cli 自动认证尝试，而 Redis 默认用户未配置密码；命令仍返回 key 删除结果。后续用 `EXISTS` 校验目标缓存 key 总数为 0。
- 当前浏览器中登录的迁移账号今日用量为 0，所以 UI 显示 `$0.00 / $19.00`；数据库中有当日历史用量的迁移账号已通过 API 和数据库聚合验证。

## 后续

- 后续真实请求将由 Sub2API 自己记录到 `usage_logs`。
- 若需要展示 yui.web 历史趋势，单独设计 legacy usage 只读归档，不要污染 Sub2API 原生 `usage_logs`。
- 如果之后要迁真实邮箱，需要同步更新 `users.email`、`users.username` 和 `auth_identities.provider_subject`。

