# yui.web 已发 API Key 导入 Sub2API 执行计划

## 目标

按 `20260618-103811-yuiweb-legacy-api-key-import-design_CN.md` 的口径，把 yui.web `orders` 中 15 个已发 API Key 导入 Sub2API，让老用户继续使用原来的 `sk-*` Key。

## 边界

- 导入范围：`orders` 中 15 个已发 Key。
- 不导入：yui.web `api_keys.status='unused'` 的库存 Key。
- 不导入：yui.web 历史 `usage_events` 到 Sub2API `usage_logs`。
- 不迁移：yui.web 余额/授信到 Sub2API `users.balance`。
- 不打印：完整 API Key、`SHOP_API_KEY_ENCRYPTION_SECRET`、内部 token。

## 映射

- 用户：`orders.phone -> users.email = <phone>@phone.com`
- Key：yui.web 加密 Key 解密后写入 Sub2API `api_keys.key`
- Key 名称：`yui.web legacy key <preview>`
- active 订阅：
  - `sub_29_daily_19_usd -> codex-pool`
  - `sub_39_daily_29_usd -> codex-pool-29-usd`
  - `sub_59_daily_49_usd -> codex-pool-49-usd`
- 非 active / 无订阅旧 Key：建立人工迁移订阅，绑定最低档 `codex-pool`，到期时间使用 `orders.expires_at`
- Key 自身不设置 `expires_at`，权益到期由 `user_subscriptions.expires_at` 判断

## 执行步骤

1. 编写幂等迁移脚本 `scripts/migrate-yuiweb-legacy-api-keys.mjs`。
2. dry-run：
   - 解密 15 个订单 Key。
   - 校验 preview/hash。
   - 校验 15 个 `<phone>@phone.com` 用户存在。
   - 校验目标 group 存在。
   - 校验 Sub2API 现有 Key 冲突。
   - 输出脱敏预览和分类数量。
3. 正式导入前备份：
   - Sub2API PostgreSQL dump。
   - yui.web SQLite copy。
4. 正式导入：
   - upsert 15 个 `user_subscriptions` 权益。
   - insert/upsert 15 个 `api_keys`。
   - 清理 Redis `apikey:auth:*`、`billing:sub:*`、`apikey:rate:*` 相关缓存。
5. 验证：
   - 数据库中 15 个旧 Key 都能映射到 Sub2API 用户和 group。
   - 15 个用户都有 active 订阅权益。
   - 使用一个迁移 Key 请求本地 `/v1/models`。
   - 如可行，使用一个迁移 Key 请求本地 `/v1/chat/completions` 并确认 `usage_logs` / `billing_usage_entries` 增加。

## 风险控制

- `--apply` 之外默认 dry-run。
- dry-run 与 apply 使用同一套 staging 数据。
- 如果发现缺失用户、缺失 group、不同用户 Key 冲突、解密失败，正式导入前中止。
- 脚本输出只包含脱敏手机号、key preview、hash 前缀和数量。
