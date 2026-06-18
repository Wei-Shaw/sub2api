# yui.web 用户与 Sub2API 注册 / 登录迁移判断

## 时间

2026-06-18 09:56 JST

## 问题

用户询问 `yui.web/shop` 现有用户登录注册数据是否能直接迁移或对接到当前本地 Sub2API 登录页，并询问当前 `http://127.0.0.1:18080/register` 为什么显示“注册功能暂时关闭”。

## 已确认事实

### Sub2API 注册关闭原因

当前 Sub2API 公共设置返回：

```json
{
  "registration_enabled": false,
  "site_name": "Sub2API",
  "invitation_code_enabled": false,
  "promo_code_enabled": true,
  "registration_email_suffix_whitelist": []
}
```

`settings` 表中当前没有 `registration_enabled` 记录。Sub2API 代码在 `SettingService.IsRegistrationEnabled()` 中采用安全默认：设置不存在或查询失败时默认关闭注册。因此页面不是故障，而是安全默认关闭。

代码事实：

- `backend/internal/service/domain_constants.go`：`registration_enabled`
- `backend/internal/service/setting_service.go`：设置缺失时返回 `false`
- `frontend/src/views/auth/RegisterView.vue`：读取 public settings 后展示关闭提示

### yui.web/shop 用户库

真实库：

```text
/Users/wujianxiang/CodeSpace/yui.web/data/shop.sqlite
```

关键表：

- `users(phone, created_at, password_hash, password_created_at, updated_at)`
- `user_sessions(token_hash, phone, created_at, expires_at, revoked_at, csrf_token_hash)`
- `api_keys(...)`
- `account_subscriptions(...)`
- `subscription_orders(...)`
- `account_balances(...)`
- `api_usd_charge_records(...)`

脱敏统计：

- `users`：21
- 有密码用户：20
- `account_subscriptions`：13
- active 订阅：12
- `api_usd_charge_records`：1429

### 密码哈希不兼容

`yui.web` 密码哈希格式：

```text
scrypt$N$r$p$salt$hash
```

Sub2API 密码哈希格式：

```text
bcrypt
```

因此不能简单复制 `password_hash` 到 Sub2API 的 `users.password_hash` 后让用户用原密码登录。若要保留原密码，需要 Sub2API 增加兼容 scrypt 校验和登录后重哈希迁移逻辑；否则只能迁移用户身份并要求首次登录重置密码。

### Sub2API 当前用户库

当前 Sub2API PostgreSQL `users` 只有 2 个未删除用户：

- 管理员 `admin@sub2api.local`
- 本地测试用户 `sub2api-test-local@example.com`

## 判断

不建议把 Sub2API 登录页直接接 yui.web 的 SQLite 用户表作为实时登录源。

原因：

- Sub2API 是方案 A 的唯一用户 Key / 计费 / 用量事实源，用户状态应该落在 Sub2API PostgreSQL。
- 直接跨项目读 yui.web SQLite 会产生双事实源：Sub2API 和 yui.web 都可能判断用户、订阅、余额、Key 状态。
- 两边 session、CSRF、密码哈希、用户标识模型不同：yui.web 用手机号，Sub2API 用 email/username。
- 未来公网部署时，Sub2API 容器直接读宿主机 yui.web SQLite 会增加路径、锁、备份和并发风险。

## 推荐方案

推荐一次性迁移：

1. 从 `yui.web/data/shop.sqlite` 读取用户、订阅和必要权益。
2. 在 Sub2API 中创建对应用户。
3. 用户 email 可采用可控映射，例如 `手机号@phone.local` 或真实手机号字段另存 notes / identity metadata；不在公开文档中打印完整手机号。
4. 对有 active 订阅的 yui 用户，迁移成 Sub2API 对应的余额、分组、订阅或平台配额策略。
5. 不迁移旧 yui.web API key 到用户侧继续使用；改为生成 Sub2API API Key。
6. 密码处理二选一：
   - 简单稳妥：要求用户通过重置/首次设置密码进入 Sub2API。
   - 无感迁移：Sub2API 临时支持 yui.web scrypt hash，用户首次成功登录后改写为 bcrypt。

## Sub2API 注册开放方式

可以通过后台 `管理 -> 设置 -> 安全 -> 注册设置 -> 开启注册` 打开。

也可以用 DB 设置，但这会绕过后台审计，不作为首选：

```sql
INSERT INTO settings (key, value, updated_at)
VALUES ('registration_enabled', 'true', now())
ON CONFLICT (key) DO UPDATE
SET value = excluded.value,
    updated_at = excluded.updated_at;
```

公网开放注册前要先决定是否启用邀请码、邮箱验证、邮箱后缀白名单或 Turnstile。当前 `invitation_code_enabled=false`，如果直接打开注册，理论上任何人都能注册。

## 后续建议

先不要“实时对接 yui.web 登录库”。下一步应做一份迁移设计：

- 明确用户标识映射：手机号如何变成 Sub2API 用户 email/username。
- 明确权益映射：active 订阅、每日额度、加量包和历史扣费是否迁移。
- 明确密码方案：重置密码还是兼容 scrypt 首登迁移。
- 明确上线切换：yui.web/shop 退为展示和跳转，不再产生新用户事实源。
