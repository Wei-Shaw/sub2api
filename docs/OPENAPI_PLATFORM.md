# OpenAPI 平台数据集成接口

> M2M（Machine-to-Machine）数据集成接口，供外部平台（商城、CRM、卡密签发系统等）通过 HTTP 调用 sub2api 创建用户、调整余额、管理 LLM API Key、查询用量。

**Base path:** `/api/v1/openapi`

**鉴权:** 复用 sub2api 现有的 Admin API Key 体系，HTTP header `x-api-key: <admin_api_key>`。

---

## 鉴权与凭证

所有 `/api/v1/openapi/*` 路由均经过 `adminAuth` 中间件，要求请求带上 Admin API Key：

```
x-api-key: <admin_api_key>
```

Admin API Key 在管理员后台 **系统设置 → Admin API Key** 处生成 / 轮换，也可通过 admin 接口管理：

- `GET    /api/v1/admin/settings/admin-api-key` — 查看
- `POST   /api/v1/admin/settings/admin-api-key/regenerate` — 轮换
- `DELETE /api/v1/admin/settings/admin-api-key` — 删除

**注意:** 该 key 与 admin 后台共用，请妥善保管；生产环境强烈建议在网关侧加 IP 白名单。

---

## 接口清单

| Method | Path | 说明 |
|---|---|---|
| POST   | `/users` | 创建用户（按 email 幂等） |
| GET    | `/users/:email` | 查询用户基础信息 + 余额 |
| PATCH  | `/users/:email/balance` | 调整余额（set / add；external_op_id 幂等） |
| POST   | `/users/:email/keys` | 给指定用户生成 LLM API Key |
| GET    | `/users/:email/keys` | 列出指定用户的 keys |
| PATCH  | `/users/:email/keys/:key_id` | 修改 key（quota / 限流 / 状态） |
| DELETE | `/users/:email/keys/:key_id` | 删除 key |
| GET    | `/users/:email/usage` | 用户消费明细（分页） |
| GET    | `/users/:email/usage/stats` | 用户消费聚合（含 token / cost / 请求数） |

`:email` 是 path 参数，需要 URL-encode：`alice@example.com` → `alice%40example.com`。

---

## 接口详情

### 1. 创建用户

```
POST /api/v1/openapi/users
```

**Request:**

```json
{
  "email": "alice@example.com",
  "external_user_id": "shop-mall-uid-1234",
  "initial_balance": 50.0,
  "key_name": "default",
  "group_id": null
}
```

| 字段 | 必填 | 默认 | 说明 |
|---|---|---|---|
| `email` | 是 | — | 用户 email |
| `external_user_id` | 否 | — | 外部平台的用户号（仅做请求标识，不持久化） |
| `initial_balance` | 否 | `0` | 建号即送的余额 |
| `key_name` | 否 | `"default"` | 自动生成的首个 LLM key 名称 |
| `group_id` | 否 | `null` | 首个 key 绑定的分组 id |

**Response（首次成功，`first_time=true`）:**

```json
{
  "data": {
    "user_id": 8421,
    "email": "alice@example.com",
    "status": "active",
    "balance": 50.0,
    "api_key": "sk-1234abcd...64hex",
    "api_key_id": 17832,
    "first_time": true
  }
}
```

**Response（email 已存在，幂等重放）:**

```json
{
  "data": {
    "user_id": 8421,
    "email": "alice@example.com",
    "status": "active",
    "balance": 80.0,
    "first_time": false
  }
}
```

幂等重放不再返回 `api_key` 明文（首次响应丢失即不可恢复）。

**注意:** 用户内部生成的密码不会返回；如该用户后续需要登录 sub2api Web，需走"忘记密码"邮件流程。

---

### 2. 查询用户

```
GET /api/v1/openapi/users/:email
```

**Response:**

```json
{
  "data": {
    "user_id": 8421,
    "email": "alice@example.com",
    "status": "active",
    "role": "user",
    "balance": 80.0,
    "total_recharged": 80.0,
    "concurrency": 5,
    "created_at": "2026-05-27T07:00:00Z"
  }
}
```

**错误:** email 不存在 → `404 user_not_found`

---

### 3. 调整余额

```
PATCH /api/v1/openapi/users/:email/balance
```

**Request:**

```json
{
  "external_op_id": "shop-order-99812",
  "op_type": "add",
  "amount": 30.0,
  "note": "redeem-card-ABCD-1234"
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `external_op_id` | 是 | 外部平台幂等键。同一 `external_op_id` 只能成功一次 |
| `op_type` | 是 | `"set"` 直接设置 / `"add"` 增量加 |
| `amount` | 是 | 操作金额，必须 ≥ 0 |
| `note` | 否 | 审计备注（如卡密码、订单号等） |

**Response:**

```json
{
  "data": {
    "operation_id": 5566,
    "user_id": 8421,
    "email": "alice@example.com",
    "op_type": "add",
    "amount": 30.0,
    "balance_before": 50.0,
    "balance_after": 80.0,
    "idempotent_replay": false
  }
}
```

**幂等行为:**

- 同 `external_op_id` 之前 `succeeded` → 返回原 `balance_before` / `balance_after`，`idempotent_replay=true`，**不重复扣加**
- 同 `external_op_id` 仍在 `pending` → `409 operation_pending`（短退避后重试或重查）
- 同 `external_op_id` 之前 `failed` → `409 operation_failed`，必须用新的 `external_op_id` 重试

**其他错误:**

- email 不存在 → `404 user_not_found`
- `op_type=set` 且 `amount` 会导致负余额 → `422 insufficient_balance`
- `op_type` 不是 `set` / `add` 或 `amount < 0` → `400 invalid_request`

---

### 4. 创建用户的 LLM API Key

```
POST /api/v1/openapi/users/:email/keys
```

**Request:** 与现有 `POST /api/v1/keys` 完全一致：

```json
{
  "name": "for-claude-cli",
  "group_id": null,
  "custom_key": null,
  "expires_in_days": 365,
  "quota": 100.0,
  "rate_limit_5h": null,
  "rate_limit_1d": null,
  "rate_limit_7d": null,
  "ip_whitelist": [],
  "ip_blacklist": []
}
```

**Response:** 标准 APIKey DTO，**含 `key` 明文（仅首次返回）**。

---

### 5. 列出用户的 keys

```
GET /api/v1/openapi/users/:email/keys?status=active
```

支持 query `status=active|disabled` 过滤。

**Response:** APIKey 列表，`key` 字段是脱敏前缀（不含明文）。

---

### 6. 修改 key

```
PATCH /api/v1/openapi/users/:email/keys/:key_id
```

**Request:** 支持改 `name` / `group_id` / `status` / `quota` / `expires_at` / `rate_limit_*` / `ip_whitelist` / `ip_blacklist`、以及 `reset_quota` / `reset_rate_limit_usage` 两个动作。

**注意:** 接口会校验 `key.user_id` 与 path 里 email 解析出的 user_id 一致，否则拒绝（防越权）。

---

### 7. 删除 key

```
DELETE /api/v1/openapi/users/:email/keys/:key_id
```

**Response:** `{"data": {"deleted": true}}`

---

### 8. 用户消费明细

```
GET /api/v1/openapi/users/:email/usage?page=1&page_size=50&model=claude-sonnet-4-6&api_key_id=17832
```

| Query | 默认 | 说明 |
|---|---|---|
| `page` | 1 | 页码 |
| `page_size` | 50 | 每页条数，最大 200 |
| `model` | — | 按模型过滤 |
| `api_key_id` | — | 按 key id 过滤 |

**Response:** 分页 UsageLog 列表，含 token 数、cost、模型、对应 api_key_id 等。

---

### 9. 用户消费聚合

```
GET /api/v1/openapi/users/:email/usage/stats?start_at=2026-05-20T00:00:00Z&end_at=2026-05-27T00:00:00Z
```

| Query | 默认 | 说明 |
|---|---|---|
| `start_at` | 7 天前 | ISO 8601 |
| `end_at` | now | ISO 8601 |

时间窗口最大 90 天，超过返回 `400 invalid_request`。

**Response:**

```json
{
  "data": {
    "total_requests": 812,
    "total_input_tokens": 1100000,
    "total_output_tokens": 250000,
    "total_cache_tokens": 750000,
    "total_tokens": 2100000,
    "total_cost": 12.65,
    "total_actual_cost": 12.65,
    "average_duration_ms": 1234.5
  }
}
```

---

## 错误响应

统一格式：

```json
{
  "code": 404,
  "message": "user_not_found"
}
```

| HTTP | `message` | 说明 |
|---|---|---|
| 400 | `invalid_request: ...` | 入参校验失败 |
| 401 | — | Admin API Key 缺失或错误 |
| 404 | `user_not_found` / `key_not_found` | 找不到目标 |
| 409 | `operation_pending` | 同 `external_op_id` 仍在处理 |
| 409 | `operation_failed` | 同 `external_op_id` 之前失败，必须换新 id 重试 |
| 422 | `insufficient_balance` | set/add 会使余额变负 |
| 500 | — | 服务器内部错误 |

---

## 安全建议

1. **强制 HTTPS** — 所有调用必须走 TLS，避免 Admin API Key 与 sk-key 明文外泄
2. **IP 白名单** — 在你的网关 / Nginx 配置中限制 `/api/v1/openapi/*` 仅接受外部平台服务器 IP
3. **凭证最小化暴露** — 创建用户的 `api_key` 仅在首次响应中返回，外部平台应"读取 → 传递 → 立即丢弃"，不持久化
4. **`external_op_id` 命名规范** — 推荐前缀化（如 `shop-`、`refund-`），避免不同业务流水冲突
5. **轮换** — 定期通过 `POST /api/v1/admin/settings/admin-api-key/regenerate` 轮换 Admin API Key

---

## 完整示例（curl）

```bash
ADMIN_KEY="sk-admin-..."
BASE="https://your-domain.com/api/v1"

# 1. 创建用户 + 初始充值 + 首个 LLM key
curl -X POST "$BASE/openapi/users" \
  -H "x-api-key: $ADMIN_KEY" -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","initial_balance":50}'

# 2. 查询
curl -H "x-api-key: $ADMIN_KEY" "$BASE/openapi/users/alice%40example.com"

# 3. 加余额（卡密兑付到账后）
curl -X PATCH -H "x-api-key: $ADMIN_KEY" -H "Content-Type: application/json" \
  "$BASE/openapi/users/alice%40example.com/balance" \
  -d '{"external_op_id":"shop-order-99812","op_type":"add","amount":30,"note":"redeem-card-ABCD"}'

# 4. 生成额外 key
curl -X POST -H "x-api-key: $ADMIN_KEY" -H "Content-Type: application/json" \
  "$BASE/openapi/users/alice%40example.com/keys" \
  -d '{"name":"production","quota":100,"expires_in_days":365}'

# 5. 查用量聚合
curl -H "x-api-key: $ADMIN_KEY" \
  "$BASE/openapi/users/alice%40example.com/usage/stats?start_at=2026-05-20T00:00:00Z&end_at=2026-05-27T00:00:00Z"
```
