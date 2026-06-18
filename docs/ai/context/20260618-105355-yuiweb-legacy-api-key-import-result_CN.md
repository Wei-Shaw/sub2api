# yui.web 已发 API Key 导入 Sub2API 结果

## 结果

- 已导入 yui.web `orders` 中 15 个已发 API Key。
- 15 个 Key 全部绑定到 Sub2API 已迁移用户。
- 15 个 Key 全部绑定到 `codex-pool`。
- 12 个 Key 复用上一轮已迁 active subscription。
- 3 个没有 active subscription 的旧 Key 已补人工迁移订阅，到期时间使用对应 `orders.expires_at`。
- 未导入 yui.web `api_keys.status='unused'` 的 15 个库存 Key。
- 未导入 yui.web 历史 `usage_events` 到 Sub2API `usage_logs`。
- 未迁移 yui.web 余额/授信到 Sub2API `users.balance`。

## 备份

- Sub2API PostgreSQL 备份：
  - `/Users/wujianxiang/CodeSpace/sub2api/.tmp-sub2api-before-legacy-api-key-import-20260618-015125.dump`
- yui.web SQLite 备份：
  - `/Users/wujianxiang/CodeSpace/sub2api/.tmp-yuiweb-shop-before-legacy-api-key-import-20260618-015125.sqlite`

备注：上述备份文件名由脚本初版使用 UTC 生成；脚本已修正为本地时间格式。

## Dry-run 对账

- 计划迁移 Key：15。
- 可解密 Key：15。
- Sub2API 用户匹配：15。
- group 匹配：15。
- Key 冲突：0。
- 软删除 Key 冲突：0。
- active subscription 来源：12。
- 人工订单订阅来源：3。
- 当日用量合计：`2.805383 USD`。
- 当前周期用量合计：`50.189679 USD`。

## 正式导入

- `api_keys` 中 legacy Key 记录：15。
- legacy Key 对应 active 订阅权益：15。
- 人工迁移订阅记录：3。
- Redis 缓存清理：
  - 执行删除命令：45。
  - 删除命中数：0。

第一次 `--backup --apply` 已提交数据库事务，但脚本把 `psql` 的 `BEGIN/COMMIT/SELECT` 命令标签一起当 JSON 解析而报错；确认数据库已写入后，已修复脚本只解析最后一行 JSON，并幂等复跑 `--apply` 完成缓存清理和结构化输出。

## API 验证

- 使用迁移 Key 请求本地 `http://127.0.0.1:18080/v1/models`：
  - HTTP 200。
  - 返回模型数量：10。
  - 示例首个模型：`gpt-5.5`。
- 使用同一个迁移 Key 请求本地 `http://127.0.0.1:18080/v1/chat/completions`：
  - HTTP 200。
  - `usage_logs` 增量：1。
  - 最后一条验证请求：
    - `model=gpt-5.5`
    - `total_cost=0.0016850000`
    - `actual_cost=0.0016850000`
    - `billing_type=1`
    - `subscription_id=2`
  - 对应订阅用量已从 `1.988370` 增至 `1.990055`。

## 注意

- 当前代码路径没有向 `billing_usage_entries` 写入记录；本次验证中该表增量为 0。
- 当前 Sub2API 的订阅扣费事实源实际体现在：
  - `usage_logs.subscription_id`
  - `usage_logs.billing_type=1`
  - `user_subscriptions.daily_usage_usd`
  - `user_subscriptions.weekly_usage_usd`
  - `user_subscriptions.monthly_usage_usd`
- 以后如果要强依赖 `billing_usage_entries` 做报表或审计，需要单独修正运行时代码路径，不要把这次 API Key 迁移和账务窄表改造混在一起。

## 使用方式

老用户可继续使用原 yui.web 发放的 Key：

```text
Base URL: https://api.aaccx.pw/v1
API Key: 原 yui.web 已发给用户的 sk-...
```

本地验证入口：

```text
Base URL: http://127.0.0.1:18080/v1
```
