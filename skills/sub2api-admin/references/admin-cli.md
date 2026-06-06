# Sub2API Admin Reference

## Environment

```bash
export SUB2API_BASE_URL='https://your-sub2api-host'
export SUB2API_ADMIN_API_KEY='<admin api key>'
```

后台鉴权只使用 `x-api-key`。如果返回 `INVALID_ADMIN_KEY`，重新生成管理员 API Key。

## CLI

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js <command>
```

## Accounts

### 只读

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts list --page-size 20
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts list --search outlook --platform openai --type oauth --status active
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts get 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts usage 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts stats 40 --days 30
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts today-stats 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts batch-today-stats --ids 40,39
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts models 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts temp-unschedulable 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts antigravity-default-model-mapping
```

`accounts export` 会包含账号凭据和 token，建议写入文件，不要直接刷屏：

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts export --ids 40,39 --file accounts-export.json
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts export --platform openai --type oauth --include-proxies false --file accounts-export.json
```

### 单账号写入

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts create --file account.json
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts update 40 --json '{"concurrency":20}'
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts set-status 40 active
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts set-schedulable 40 true
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts clear-error 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts clear-rate-limit 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts recover-state 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts reset-quota 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts refresh 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts test 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts sync-models 40
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts apply-oauth 40 --file credentials.json
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts reset-temp-unschedulable 40
```

### 删除与清理

删除前先列出目标账号名和 ID。

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts delete 25
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts keep-only --name 'target@example.com'
```

### 批量写入

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts batch-create --file accounts.json
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts batch-update-credentials --file payload.json
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts bulk-update --ids 40,39 --json '{"concurrency":10,"priority":2}'
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts batch-refresh --ids 40,39
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts batch-clear-error --ids 40,39
```

`bulk-update` 可覆盖页面“批量更新”的字段，payload 由后台表单字段决定，例如 `base_url`、`model_mapping`、`group_ids`、`proxy_id`、`concurrency`、`priority`、`rate_multiplier`、`status`、`compact_mode` 等。更新前先用 `accounts get <id>` 确认字段名。

### 导入

通用后台导入：

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts import-data --file accounts-export.json
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts import-codex-session --file payload.json
```

CRS 同步：

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts crs-preview --file payload.json
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts crs-sync --file payload.json
```

旧版 JSON 导入仍可用，会把模板账号的配置复制给导入账号：

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js accounts import-json \
  --file /path/accounts.json \
  --template-name 'template@example.com' \
  --dry-run
```

复制字段：

- `concurrency`
- `priority`
- `group_ids`
- `credentials.model_mapping`

## Groups And Proxies

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js groups all
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js proxies all
```

## Error Rules And TLS Profiles

对应账号页顶部“错误透传规则”和“TLS 指纹模板”。

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js error-rules list
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js error-rules get 1
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js error-rules create --file rule.json
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js error-rules update 1 --json '{"enabled":true}'
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js error-rules toggle 1 false
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js error-rules delete 1

node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js tls-profiles list
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js tls-profiles get 1
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js tls-profiles create --file profile.json
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js tls-profiles update 1 --file profile.json
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js tls-profiles delete 1
```

## Raw Admin API

未封装或新版本后台接口可用 `api` 直通。路径可写 `/admin/...` 或 `/api/v1/admin/...`。

```bash
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js api GET /admin/groups/all
node ~/.agents/skills/sub2api-admin/scripts/sub2api-admin.js api POST /admin/accounts/bulk-update \
  --json '{"account_ids":[40],"concurrency":10}'
```

## Confirmed Admin Endpoints

- `GET /api/v1/admin/accounts`
- `GET /api/v1/admin/accounts/:id`
- `POST /api/v1/admin/accounts`
- `PUT /api/v1/admin/accounts/:id`
- `DELETE /api/v1/admin/accounts/:id`
- `POST /api/v1/admin/accounts/check-mixed-channel`
- `GET /api/v1/admin/accounts/:id/usage`
- `GET /api/v1/admin/accounts/:id/stats`
- `GET /api/v1/admin/accounts/:id/today-stats`
- `POST /api/v1/admin/accounts/today-stats/batch`
- `POST /api/v1/admin/accounts/:id/schedulable`
- `POST /api/v1/admin/accounts/:id/test`
- `POST /api/v1/admin/accounts/:id/refresh`
- `POST /api/v1/admin/accounts/:id/apply-oauth-credentials`
- `POST /api/v1/admin/accounts/:id/clear-error`
- `POST /api/v1/admin/accounts/:id/clear-rate-limit`
- `POST /api/v1/admin/accounts/:id/recover-state`
- `POST /api/v1/admin/accounts/:id/reset-quota`
- `GET /api/v1/admin/accounts/:id/temp-unschedulable`
- `DELETE /api/v1/admin/accounts/:id/temp-unschedulable`
- `GET /api/v1/admin/accounts/:id/models`
- `POST /api/v1/admin/accounts/:id/models/sync-upstream`
- `POST /api/v1/admin/accounts/batch`
- `POST /api/v1/admin/accounts/batch-update-credentials`
- `POST /api/v1/admin/accounts/bulk-update`
- `POST /api/v1/admin/accounts/batch-refresh`
- `POST /api/v1/admin/accounts/batch-clear-error`
- `GET /api/v1/admin/accounts/data`
- `POST /api/v1/admin/accounts/data`
- `POST /api/v1/admin/accounts/import/codex-session`
- `POST /api/v1/admin/accounts/sync/crs/preview`
- `POST /api/v1/admin/accounts/sync/crs`
- `GET /api/v1/admin/accounts/antigravity/default-model-mapping`
- `GET /api/v1/admin/groups/all`
- `GET /api/v1/admin/proxies/all`
- `GET /api/v1/admin/error-passthrough-rules`
- `GET /api/v1/admin/error-passthrough-rules/:id`
- `POST /api/v1/admin/error-passthrough-rules`
- `PUT /api/v1/admin/error-passthrough-rules/:id`
- `DELETE /api/v1/admin/error-passthrough-rules/:id`
- `GET /api/v1/admin/tls-fingerprint-profiles`
- `GET /api/v1/admin/tls-fingerprint-profiles/:id`
- `POST /api/v1/admin/tls-fingerprint-profiles`
- `PUT /api/v1/admin/tls-fingerprint-profiles/:id`
- `DELETE /api/v1/admin/tls-fingerprint-profiles/:id`

## Notes

- 线上写入前先只读核对目标集合。
- 导出结果包含敏感凭据，优先使用 `--file`。
- `PUT /admin/accounts/:id` 和 `bulk-update` 接受宽松请求体，字段名不确定时先用 `accounts get` 或后台页面确认。
