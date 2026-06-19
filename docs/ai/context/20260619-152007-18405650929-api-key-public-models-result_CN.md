# 18405650929 用户 API Key 与公网模型验证记录

## 背景

- 查询目标用户：`18405650929@phone.com`
- 操作类型：只读数据库查询 + 公网 `/v1/models` 验证
- 安全约束：不记录完整 API Key、数据库密码、JWT secret 或内部 token

## 查询方法

- 运行态服务：Docker Desktop 中的 `sub2api` / `sub2api-postgres` / `sub2api-redis`
- Sub2API 公网入口：
  - `https://aaccx.pw/v1/models`
  - `https://api.aaccx.pw/v1/models`
- 数据库：`sub2api-postgres` 容器内 PostgreSQL `sub2api`
- API Key 使用 shell 变量从数据库读取后直接传给 `curl`，未在终端输出完整值

## 数据库结果

- 用户存在：
  - `users.id=18`
  - `email=18405650929@phone.com`
  - `username=18405650929@phone.com`
  - `role=user`
  - `status=active`
  - `created_at=2026-06-18 09:13:20.931974+08`
- 已创建 API Key：
  - `api_keys.id=15`
  - `user_id=18`
  - `name=yui.web legacy key sk-yui-l...OQjSJH`
  - `group_id=2`
  - `group_name=codex-pool-19-usd`
  - `status=active`
  - `masked_key=sk-yui-l...OQjSJH`
  - `last_used_at=2026-06-18 11:37:33.856742+08`
- active 订阅存在：
  - `user_subscriptions.id=12`
  - `group_id=2`
  - `group_name=codex-pool-19-usd`
  - `status=active`
  - `starts_at=2026-06-17 16:06:37.531+08`
  - `expires_at=2026-07-17 16:06:37.531+08`
  - `daily_usage_usd=0.0020400000`
  - `weekly_usage_usd=0.0020400000`
  - `monthly_usage_usd=0.0020400000`

## 公网验证结果

- `https://aaccx.pw/v1/models`
  - HTTP status：`200`
  - 模型数量：`10`
  - 示例模型：`gpt-5.5`、`gpt-5.4`、`gpt-5.4-mini`、`gpt-5.3-codex`、`gpt-5.3-codex-spark`、`codex-auto-review`、`gpt-5.2`、`gpt-image-1`
- `https://api.aaccx.pw/v1/models`
  - HTTP status：`200`
  - 模型数量：`10`
  - 示例模型同上

## 结论

- `18405650929@phone.com` 已有 active API Key。
- 该 Key 绑定 `codex-pool-19-usd`，且对应订阅 active。
- 使用该 Key 通过两个公网入口访问模型列表均成功，当前公网模型读取链路可用。
