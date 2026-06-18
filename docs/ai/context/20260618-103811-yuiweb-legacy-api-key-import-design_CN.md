# yui.web 已发 API Key 导入 Sub2API 设计

## 背景

当前公网入口已经切到 Sub2API：

```text
公网用户
  -> https://api.aaccx.pw/v1
  -> Sub2API 127.0.0.1:18080
  -> CLIProxyAPI 127.0.0.1:8317
  -> 本地账号池
```

用户提出新的迁移口径：yui.web 数据库里已经给用户一一对应发放过 API Key，希望这些旧 Key 可以直接在 Sub2API 继续使用，降低用户切换成本。

本设计只记录方案，不执行数据迁移。

## 当前只读核查结果

yui.web SQLite：`/Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite`

- `api_keys` 总数：30。
- `api_keys.status='used'`：15。
- `api_keys.status='unused'`：15。
- `orders` 中带 API Key 的订单：15。
- 带 API Key 的不同手机号：15。
- `account_subscriptions.status='active'`：12。
- 15 个带 API Key 的用户中，有 3 个当前没有 active subscription。
- yui.web 的 API Key 明文不应直接打印；库内有 `api_key_hash`、`api_key_preview`、`api_key_ciphertext`、`api_key_nonce`。
- 本机 yui.web `.env` 存在 `SHOP_API_KEY_ENCRYPTION_SECRET`，技术上可以解密旧 Key 后导入 Sub2API；迁移脚本不得把完整 Key 写入日志或文档。

Sub2API PostgreSQL：

- 迁移来的手机号用户已经存在，登录名格式为 `<手机号>@phone.com`。
- `api_keys.key` 是明文唯一字段，支持导入用户已有 Key。
- `api_keys.group_id` 可绑定订阅型 group。
- 对订阅型 group，Sub2API 创建或绑定 Key 时要求用户有对应 active subscription。
- 当前订阅分组与套餐已经存在：
  - `codex-pool`：每日 19 USD。
  - `codex-pool-29-usd`：每日 29 USD。
  - `codex-pool-49-usd`：每日 49 USD。

## 决策

按用户要求，迁移范围改为：

```text
yui.web orders 中 15 个已有 API Key 的用户全部导入 Sub2API
```

不是只迁 12 个 active subscription 用户。

导入后的用户使用方式：

```text
Base URL: https://api.aaccx.pw/v1
API Key: 原 yui.web 已发给用户的 sk-...
```

用户不需要重新登录 Sub2API 创建新 Key；旧客户端只需要把 Base URL 从旧入口改成 Sub2API 公网入口。

## 数据映射

### 用户映射

- yui.web：`orders.phone`
- Sub2API：`users.email = <phone>@phone.com`

如果 `orders.phone` 在 Sub2API 中找不到对应用户，应中止该用户迁移并输出脱敏错误，不自动创建未知用户。

### API Key 映射

- yui.web 明文 Key 来源：
  - 优先使用 `orders.api_key_ciphertext` / `orders.api_key_nonce` 解密。
  - 若旧数据存在未加密 `orders.api_key` 明文，可作为兼容读取来源。
- Sub2API 写入：
  - `api_keys.user_id`：对应 Sub2API 用户 ID。
  - `api_keys.key`：解密得到的旧 Key 明文。
  - `api_keys.name`：建议为 `yui.web legacy key` 或包含脱敏 preview 的中文名称。
  - `api_keys.group_id`：按套餐绑定到对应订阅 group。
  - `api_keys.status`：默认 `active`。
  - `api_keys.created_at`：可使用 yui.web `orders.redeemed_at` 或迁移执行时间；推荐保留原兑换时间。
  - `api_keys.expires_at`：不建议单独设置为订单过期时间，避免和订阅到期形成双重状态；以 `user_subscriptions.expires_at` 作为真实权益到期事实源。

### 套餐与订阅映射

- yui.web `sub_29_daily_19_usd` -> Sub2API `codex-pool`。
- yui.web `sub_39_daily_29_usd` -> Sub2API `codex-pool-29-usd`。
- yui.web `sub_59_daily_49_usd` -> Sub2API `codex-pool-49-usd`。

对 12 个 active subscription 用户：

- 迁移或 upsert `user_subscriptions`。
- `starts_at` 使用 yui.web `account_subscriptions.started_at`。
- `expires_at` 使用 yui.web `account_subscriptions.expires_at`。
- `status='active'`。
- `daily_usage_usd` 使用 yui.web 当日已扣套餐额度聚合值。
- 迁移后清理对应 Redis 订阅计费缓存。

对 3 个有旧 API Key 但没有 active subscription 的用户：

- 仍按本次口径导入旧 Key。
- 不应默认给无限额度。
- 推荐建立一条人工迁移订阅，使用该用户 `orders.expires_at` 作为 `user_subscriptions.expires_at`，group 默认按订单产品映射到 `codex-pool`。
- 迁移记录中必须单独列出这 3 个用户的脱敏手机号与 key preview，便于人工复核。

## 为什么不能只导入 api_keys

Sub2API 的核心事实源不是单张 Key 表，而是：

- `api_keys`：认证用户身份和绑定 group。
- `user_subscriptions`：判断订阅权益是否有效。
- `usage_logs` / `billing_usage_entries`：记录新请求和扣费。
- Redis 订阅缓存：运行时加速额度判断。

如果只导入旧 Key，不补订阅或 group：

- Key 可能能通过认证，但没有正确订阅额度。
- 订阅型 group 绑定会被服务端拒绝。
- 请求可能走到默认 group，造成计费和模型池混乱。
- 用户当天额度可能被重置，造成多用。

## 执行边界

本阶段只迁移会影响用户继续使用的事实源：

- 15 个旧 API Key。
- 15 个 Key 对应的 Sub2API 用户绑定关系。
- 15 个用户的可用订阅权益。
- 当日已用套餐额度。
- Redis 订阅缓存清理。

本阶段不迁移：

- yui.web 历史 `usage_events` 到 Sub2API `usage_logs`。
- yui.web 历史人民币余额、授信和账单到 Sub2API `users.balance`。
- yui.web 未发放的 15 个 unused API Key 库存。

未发放库存 Key 没有用户依赖，继续保留在 yui.web 归档即可；Sub2API 后续新用户使用自己的 Key 生成能力。

## 安全要求

- 迁移脚本不得打印完整 API Key。
- 文档和 AGENTS.md 不记录完整 API Key、内部 token、HMAC secret、加密 secret。
- 迁移前必须备份 yui.web SQLite、Sub2API PostgreSQL。
- 迁移前必须检查旧 Key 与 Sub2API 现有 `api_keys.key` 是否冲突。
- 迁移后必须用脱敏 preview 和数量校验，不用完整 Key 校验。
- CLIProxyAPI 内部 Key 不发给用户。

## 验收标准

迁移完成后应验证：

1. Sub2API `api_keys` 新增 15 条旧 Key 对应记录，且都绑定到正确用户。
2. 15 个用户都有可用订阅权益，12 个来自 active subscription，3 个来自人工迁移订阅。
3. 使用任一迁移 Key 请求公网 `/v1/models` 返回 HTTP 200。
4. 使用任一迁移 Key 请求公网 `/v1/chat/completions` 返回 HTTP 200。
5. 新请求写入 Sub2API `usage_logs` 与 `billing_usage_entries`。
6. 旧 yui.web 不再作为新请求链路事实源。

## 推荐下一步

先写只读 dry-run 脚本，输出：

- 将迁移的 15 个脱敏手机号。
- 每个用户的 key preview。
- 是否能解密旧 Key。
- 是否能找到 Sub2API 用户。
- 计划绑定的 group。
- 计划写入的订阅来源：active subscription 或 orders 人工迁移。
- 当日已用额度聚合值。
- 与现有 Sub2API Key 是否冲突。

dry-run 无误后，再写正式迁移脚本执行事务化导入。
