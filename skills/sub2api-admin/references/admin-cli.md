# Sub2API Admin Reference

## Environment

```bash
export SUB2API_BASE_URL='https://your-sub2api-host'
export SUB2API_ADMIN_API_KEY='<admin api key>'
export SUB2API_USER_TOKEN='<login access token>'
```

后台鉴权只使用 `x-api-key`。如果返回 `INVALID_ADMIN_KEY`，重新生成管理员 API Key。
用户侧命令使用 `auth login` 返回的 `access_token`，通过 `Authorization: Bearer <token>` 鉴权。

## CLI

以下命令都假设当前目录是这个 skill 目录。

```bash
node scripts/sub2api-admin.js <command>
```

## User Auth And Keys

登录并取得用户 token：

```bash
node scripts/sub2api-admin.js auth login --email user@example.com --password '<password>'
export SUB2API_USER_TOKEN='<access_token>'
```

用户可用分组和用户 API Key：

```bash
node scripts/sub2api-admin.js user-groups available
node scripts/sub2api-admin.js user-groups rates
node scripts/sub2api-admin.js user-keys list --page-size 20
node scripts/sub2api-admin.js user-keys get 12
node scripts/sub2api-admin.js user-keys create --name cc-max --group-id 5 --quota 0 --idempotency-key key-$(date +%s)
node scripts/sub2api-admin.js user-keys create --json '{"name":"cc-max","group_id":5,"quota":0}'
node scripts/sub2api-admin.js user-keys update 12 --json '{"status":"inactive"}'
node scripts/sub2api-admin.js user-keys delete 12
```

## Accounts

### 只读

```bash
node scripts/sub2api-admin.js accounts list --page-size 20
node scripts/sub2api-admin.js accounts list --search outlook --platform openai --type oauth --status active
node scripts/sub2api-admin.js accounts get 40
node scripts/sub2api-admin.js accounts usage 40
node scripts/sub2api-admin.js accounts stats 40 --days 30
node scripts/sub2api-admin.js accounts today-stats 40
node scripts/sub2api-admin.js accounts batch-today-stats --ids 40,39
node scripts/sub2api-admin.js accounts models 40
node scripts/sub2api-admin.js accounts temp-unschedulable 40
node scripts/sub2api-admin.js accounts antigravity-default-model-mapping
node scripts/sub2api-admin.js accounts sync-models-preview --json '{"platform":"openai","type":"api-key","api_key":"sk-..."}'
```

`accounts export` 会包含账号凭据和 token，建议写入文件，不要直接刷屏：

```bash
node scripts/sub2api-admin.js accounts export --ids 40,39 --file accounts-export.json
node scripts/sub2api-admin.js accounts export --platform openai --type oauth --include-proxies false --file accounts-export.json
```

### 单账号写入

```bash
node scripts/sub2api-admin.js accounts create --file account.json
node scripts/sub2api-admin.js accounts update 40 --json '{"concurrency":20}'
node scripts/sub2api-admin.js accounts set-status 40 active
node scripts/sub2api-admin.js accounts set-schedulable 40 true
node scripts/sub2api-admin.js accounts clear-error 40
node scripts/sub2api-admin.js accounts clear-rate-limit 40
node scripts/sub2api-admin.js accounts recover-state 40
node scripts/sub2api-admin.js accounts reset-quota 40
node scripts/sub2api-admin.js accounts refresh 40
node scripts/sub2api-admin.js accounts refresh-tier 40
node scripts/sub2api-admin.js accounts test 40
node scripts/sub2api-admin.js accounts sync-models 40
node scripts/sub2api-admin.js accounts set-privacy 40
node scripts/sub2api-admin.js accounts apply-oauth 40 --file credentials.json
node scripts/sub2api-admin.js accounts reset-temp-unschedulable 40
```

### 删除与清理

删除前先列出目标账号名和 ID。

```bash
node scripts/sub2api-admin.js accounts delete 25
node scripts/sub2api-admin.js accounts keep-only --name 'target@example.com'
```

### 批量写入

```bash
node scripts/sub2api-admin.js accounts batch-create --file accounts.json
node scripts/sub2api-admin.js accounts batch-update-credentials --file payload.json
node scripts/sub2api-admin.js accounts bulk-update --ids 40,39 --json '{"concurrency":10,"priority":2}'
node scripts/sub2api-admin.js accounts batch-refresh --ids 40,39
node scripts/sub2api-admin.js accounts batch-refresh-tier --ids 40,39
node scripts/sub2api-admin.js accounts batch-refresh-tier
node scripts/sub2api-admin.js accounts batch-clear-error --ids 40,39
```

`bulk-update` 可覆盖页面“批量更新”的字段，payload 由后台表单字段决定，例如 `base_url`、`model_mapping`、`group_ids`、`proxy_id`、`concurrency`、`priority`、`rate_multiplier`、`status`、`compact_mode` 等。更新前先用 `accounts get <id>` 确认字段名。

### 导入

通用后台导入：

```bash
node scripts/sub2api-admin.js accounts import-data --file accounts-export.json
node scripts/sub2api-admin.js accounts import-codex-session --file payload.json
```

CRS 同步：

```bash
node scripts/sub2api-admin.js accounts crs-preview --file payload.json
node scripts/sub2api-admin.js accounts crs-sync --file payload.json
```

旧版 JSON 导入仍可用，会把模板账号的配置复制给导入账号：

```bash
node scripts/sub2api-admin.js accounts import-json \
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
node scripts/sub2api-admin.js groups list --page-size 20
node scripts/sub2api-admin.js groups all
node scripts/sub2api-admin.js groups all --include-inactive
node scripts/sub2api-admin.js groups get 2
node scripts/sub2api-admin.js groups usage-summary
node scripts/sub2api-admin.js groups capacity-summary
node scripts/sub2api-admin.js groups models-list-candidates 2 --platform openai
node scripts/sub2api-admin.js groups stats 2
node scripts/sub2api-admin.js groups api-keys 2 --page-size 20
node scripts/sub2api-admin.js groups rate-multipliers 2
node scripts/sub2api-admin.js groups set-rate-multipliers 2 --json '{"entries":[{"user_id":123,"rate_multiplier":0.8}]}'
node scripts/sub2api-admin.js groups clear-rate-multipliers 2
node scripts/sub2api-admin.js groups set-rpm-overrides 2 --json '{"entries":[{"user_id":123,"rpm_override":60}]}'
node scripts/sub2api-admin.js groups clear-rpm-overrides 2
node scripts/sub2api-admin.js groups update 2 --json '{"status":"inactive"}'

node scripts/sub2api-admin.js proxies list --page-size 20
node scripts/sub2api-admin.js proxies all
node scripts/sub2api-admin.js proxies all --with-count
node scripts/sub2api-admin.js proxies get 3
node scripts/sub2api-admin.js proxies test 3
node scripts/sub2api-admin.js proxies quality-check 3
node scripts/sub2api-admin.js proxies stats 3
node scripts/sub2api-admin.js proxies accounts 3
node scripts/sub2api-admin.js proxies export --ids 3,4 --file proxies-export.json
node scripts/sub2api-admin.js proxies import-data --file proxies-export.json
node scripts/sub2api-admin.js proxies batch-create --file proxies.json
node scripts/sub2api-admin.js proxies batch-delete --ids 3,4
```

分组倍率、RPM 覆盖、代理删除等会影响真实调度；写入前先 `list`/`get` 核对目标。

## Redeem Codes

兑换码类型包括 `balance`、`concurrency`、`subscription`、`invitation`。状态常用 `unused`、`used`、`expired`。

### 只读

```bash
node scripts/sub2api-admin.js redeem-codes list --page-size 20
node scripts/sub2api-admin.js redeem-codes list --type balance --status unused --search user@example.com
node scripts/sub2api-admin.js redeem-codes get 123
node scripts/sub2api-admin.js redeem-codes stats
node scripts/sub2api-admin.js redeem-codes export --file redeem-codes.csv
```

### 生成兑换码

```bash
node scripts/sub2api-admin.js redeem-codes generate \
  --json '{"count":1,"type":"balance","value":10}' \
  --idempotency-key "redeem-generate-$(date +%s)"
```

订阅兑换码需要 `group_id` 和非零 `validity_days`：

```bash
node scripts/sub2api-admin.js redeem-codes generate \
  --json '{"count":1,"type":"subscription","value":0,"group_id":2,"validity_days":30}' \
  --idempotency-key "redeem-subscription-$(date +%s)"
```

### 创建并兑换

用于支付回调或人工充值，一步完成创建兑换码并兑换到用户。生产流程必须传稳定的 `--idempotency-key`。

```bash
node scripts/sub2api-admin.js redeem-codes create-and-redeem \
  --json '{"code":"order_123","type":"balance","value":10,"user_id":123,"notes":"manual recharge"}' \
  --idempotency-key order-123
```

### 修改与清理

写入前先 `list` 或 `get` 核对目标 ID。

```bash
node scripts/sub2api-admin.js redeem-codes batch-update --ids 123,124 --json '{"notes":"campaign A"}'
node scripts/sub2api-admin.js redeem-codes expire 123
node scripts/sub2api-admin.js redeem-codes delete 123
node scripts/sub2api-admin.js redeem-codes batch-delete --ids 123,124
```

## Error Rules And TLS Profiles

对应账号页顶部“错误透传规则”和“TLS 指纹模板”。

```bash
node scripts/sub2api-admin.js error-rules list
node scripts/sub2api-admin.js error-rules get 1
node scripts/sub2api-admin.js error-rules create --file rule.json
node scripts/sub2api-admin.js error-rules update 1 --json '{"enabled":true}'
node scripts/sub2api-admin.js error-rules toggle 1 false
node scripts/sub2api-admin.js error-rules delete 1

node scripts/sub2api-admin.js tls-profiles list
node scripts/sub2api-admin.js tls-profiles get 1
node scripts/sub2api-admin.js tls-profiles create --file profile.json
node scripts/sub2api-admin.js tls-profiles update 1 --file profile.json
node scripts/sub2api-admin.js tls-profiles delete 1
```

## Raw Admin API

未封装或新版本后台接口可用 `api` 直通。路径可写 `/admin/...` 或 `/api/v1/admin/...`。

```bash
node scripts/sub2api-admin.js api GET /admin/groups/all
node scripts/sub2api-admin.js api POST /admin/accounts/bulk-update \
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
- `POST /api/v1/admin/accounts/:id/set-privacy`
- `POST /api/v1/admin/accounts/:id/refresh-tier`
- `POST /api/v1/admin/accounts/:id/clear-error`
- `POST /api/v1/admin/accounts/:id/clear-rate-limit`
- `POST /api/v1/admin/accounts/:id/recover-state`
- `POST /api/v1/admin/accounts/:id/reset-quota`
- `GET /api/v1/admin/accounts/:id/temp-unschedulable`
- `DELETE /api/v1/admin/accounts/:id/temp-unschedulable`
- `GET /api/v1/admin/accounts/:id/models`
- `POST /api/v1/admin/accounts/:id/models/sync-upstream`
- `POST /api/v1/admin/accounts/models/sync-upstream-preview`
- `POST /api/v1/admin/accounts/batch`
- `POST /api/v1/admin/accounts/batch-update-credentials`
- `POST /api/v1/admin/accounts/batch-refresh-tier`
- `POST /api/v1/admin/accounts/bulk-update`
- `POST /api/v1/admin/accounts/batch-refresh`
- `POST /api/v1/admin/accounts/batch-clear-error`
- `GET /api/v1/admin/accounts/data`
- `POST /api/v1/admin/accounts/data`
- `POST /api/v1/admin/accounts/import/codex-session`
- `POST /api/v1/admin/accounts/sync/crs/preview`
- `POST /api/v1/admin/accounts/sync/crs`
- `GET /api/v1/admin/accounts/antigravity/default-model-mapping`
- `POST /api/v1/admin/accounts/generate-auth-url`
- `POST /api/v1/admin/accounts/generate-setup-token-url`
- `POST /api/v1/admin/accounts/exchange-code`
- `POST /api/v1/admin/accounts/exchange-setup-token-code`
- `POST /api/v1/admin/accounts/cookie-auth`
- `POST /api/v1/admin/accounts/setup-token-cookie-auth`
- `GET /api/v1/admin/groups`
- `GET /api/v1/admin/groups/all`
- `GET /api/v1/admin/groups/usage-summary`
- `GET /api/v1/admin/groups/capacity-summary`
- `PUT /api/v1/admin/groups/sort-order`
- `GET /api/v1/admin/groups/:id/models-list-candidates`
- `GET /api/v1/admin/groups/:id`
- `POST /api/v1/admin/groups`
- `PUT /api/v1/admin/groups/:id`
- `DELETE /api/v1/admin/groups/:id`
- `GET /api/v1/admin/groups/:id/stats`
- `GET /api/v1/admin/groups/:id/rate-multipliers`
- `PUT /api/v1/admin/groups/:id/rate-multipliers`
- `DELETE /api/v1/admin/groups/:id/rate-multipliers`
- `PUT /api/v1/admin/groups/:id/rpm-overrides`
- `DELETE /api/v1/admin/groups/:id/rpm-overrides`
- `GET /api/v1/admin/groups/:id/api-keys`
- `GET /api/v1/admin/proxies`
- `GET /api/v1/admin/proxies/all`
- `GET /api/v1/admin/proxies/data`
- `POST /api/v1/admin/proxies/data`
- `GET /api/v1/admin/proxies/:id`
- `POST /api/v1/admin/proxies`
- `PUT /api/v1/admin/proxies/:id`
- `DELETE /api/v1/admin/proxies/:id`
- `POST /api/v1/admin/proxies/:id/test`
- `POST /api/v1/admin/proxies/:id/quality-check`
- `GET /api/v1/admin/proxies/:id/stats`
- `GET /api/v1/admin/proxies/:id/accounts`
- `POST /api/v1/admin/proxies/batch-delete`
- `POST /api/v1/admin/proxies/batch`
- `GET /api/v1/admin/redeem-codes`
- `GET /api/v1/admin/redeem-codes/export`
- `GET /api/v1/admin/redeem-codes/stats`
- `GET /api/v1/admin/redeem-codes/:id`
- `POST /api/v1/admin/redeem-codes/generate`
- `POST /api/v1/admin/redeem-codes/create-and-redeem`
- `POST /api/v1/admin/redeem-codes/batch-update`
- `POST /api/v1/admin/redeem-codes/batch-delete`
- `POST /api/v1/admin/redeem-codes/:id/expire`
- `DELETE /api/v1/admin/redeem-codes/:id`
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

以下接口已在上游路由中确认，但当前 CLI 主要通过 `api <METHOD> <path>` 直通调用：

- `GET /api/v1/admin/dashboard/*`
- `GET|POST|PUT|DELETE /api/v1/admin/users/*`
- `GET|POST|PUT|DELETE /api/v1/admin/announcements/*`
- `POST /api/v1/admin/openai/*`
- `GET|POST /api/v1/admin/gemini/oauth/*`
- `POST /api/v1/admin/antigravity/oauth/*`
- `GET|POST|PUT|DELETE /api/v1/admin/promo-codes/*`
- `GET|PUT|POST|DELETE /api/v1/admin/settings/*`
- `GET|POST|PUT|DELETE /api/v1/admin/data-management/*`
- `GET|POST|PUT|DELETE /api/v1/admin/backups/*`
- `GET|POST /api/v1/admin/system/*`
- `GET|POST|DELETE /api/v1/admin/subscriptions/*`
- `GET|POST /api/v1/admin/usage/*`
- `GET|POST|PUT|DELETE /api/v1/admin/user-attributes/*`
- `PUT /api/v1/admin/api-keys/:id`
- `GET|POST|PUT|DELETE /api/v1/admin/ops/*`
- `GET|POST|PUT|DELETE /api/v1/admin/scheduled-test-plans/*`
- `GET|POST|PUT|DELETE /api/v1/admin/channels/*`
- `GET|POST|PUT|DELETE /api/v1/admin/channel-monitors/*`
- `GET|POST|PUT|DELETE /api/v1/admin/channel-monitor-templates/*`
- `GET|PUT|POST|DELETE /api/v1/admin/risk-control/*`
- `GET|POST|PUT|DELETE /api/v1/admin/affiliates/*`
- `POST /api/v1/auth/login`
- `GET /api/v1/groups/available`
- `GET /api/v1/groups/rates`
- `GET|POST|PUT|DELETE /api/v1/keys/*`

## Notes

- 线上写入前先只读核对目标集合。
- 导出结果包含敏感凭据，优先使用 `--file`。
- `proxies export` 可能包含代理凭据，优先使用 `--file`。
- `PUT /admin/accounts/:id` 和 `bulk-update` 接受宽松请求体，字段名不确定时先用 `accounts get` 或后台页面确认。
