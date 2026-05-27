# Sub2API OpenAPI 平台数据集成接口

> **接入参考手册** —— 供外部平台（商城、CRM、卡密签发系统等）通过 HTTP API 对接 sub2api，实现：用户开户、余额调整、LLM API Key 管理、用量查询。

---

## 目录

- [1. 概述](#1-概述)
- [2. 通用约定](#2-通用约定)
  - [2.1 Base URL & 鉴权](#21-base-url--鉴权)
  - [2.2 请求 / 响应封装格式](#22-请求--响应封装格式)
  - [2.3 URL 编码](#23-url-编码)
  - [2.4 幂等键策略](#24-幂等键策略)
  - [2.5 错误码总览](#25-错误码总览)
- [3. 接口详情](#3-接口详情)
  - [3.1 POST /users — 创建用户](#31-post-users--创建用户)
  - [3.2 GET /users/{email} — 查询用户](#32-get-usersemail--查询用户)
  - [3.3 PATCH /users/{email}/balance — 调整余额](#33-patch-usersemailbalance--调整余额)
  - [3.4 POST /users/{email}/keys — 创建 LLM Key](#34-post-usersemailkeys--创建-llm-key)
  - [3.5 GET /users/{email}/keys — 列出 Keys](#35-get-usersemailkeys--列出-keys)
  - [3.6 PATCH /users/{email}/keys/{key_id} — 修改 Key](#36-patch-usersemailkeyskey_id--修改-key)
  - [3.7 DELETE /users/{email}/keys/{key_id} — 删除 Key](#37-delete-usersemailkeyskey_id--删除-key)
  - [3.8 GET /users/{email}/usage — 消费明细](#38-get-usersemailusage--消费明细)
  - [3.9 GET /users/{email}/usage/stats — 消费聚合](#39-get-usersemailusagestats--消费聚合)
- [4. 集成场景示例](#4-集成场景示例)
  - [4.1 新用户首次充值](#41-新用户首次充值)
  - [4.2 老用户再次充值](#42-老用户再次充值)
  - [4.3 仪表板展示用户余额与用量](#43-仪表板展示用户余额与用量)
  - [4.4 幂等重试 / 网络抖动恢复](#44-幂等重试--网络抖动恢复)
- [5. 客户端 SDK 示例](#5-客户端-sdk-示例)
  - [5.1 Python](#51-python)
  - [5.2 Node.js (TypeScript)](#52-nodejs-typescript)
  - [5.3 Go](#53-go)
  - [5.4 PHP](#54-php)
- [6. 测试与联调建议](#6-测试与联调建议)
- [7. 安全建议](#7-安全建议)
- [8. FAQ](#8-faq)

---

## 1. 概述

Sub2API 是一个 AI API 网关平台，提供用户管理、API Key 分发、计费、上游 LLM 转发等能力。本套 **OpenAPI 数据集成接口**位于路由前缀 `/api/v1/openapi/`，专为"平台到平台"（M2M）场景设计：

- **谁来调用**：你的另一个业务系统（商城、卡密签发、CRM、企业内部 ERP 等），**不是终端用户**
- **能做什么**：
  - 创建 sub2api 用户（按 email 幂等）
  - 调整用户余额（set / add，带幂等键防重复扣加）
  - 代用户管理 LLM API Key（生成、列出、改、删）
  - 查询用户的消费明细与聚合统计
- **不做什么**：
  - 不处理支付流程（密卡 / 订单逻辑在外部平台）
  - 不暴露终端用户登录态（用户不直接调本接口）
  - 不返回敏感字段超过一次（如 `api_key` 明文仅首次返回）

---

## 2. 通用约定

### 2.1 Base URL & 鉴权

**Base URL**

```
https://<your-sub2api-domain>/api/v1/openapi
```

部署在内网时也可以是 `http://`，但生产环境**必须 HTTPS**。

**鉴权**

所有接口都通过 HTTP header 携带 **Admin API Key**：

```
x-api-key: <admin_api_key>
```

获取与轮换 Admin API Key：

| 操作 | Endpoint |
|---|---|
| 查看（脱敏前缀） | `GET    /api/v1/admin/settings/admin-api-key` |
| 生成或轮换 | `POST   /api/v1/admin/settings/admin-api-key/regenerate` |
| 删除 | `DELETE /api/v1/admin/settings/admin-api-key` |

> 该 key 与 sub2api admin 后台共享权限，**等同于平台级 root**。请在网关侧加 IP 白名单，定期轮换。

### 2.2 请求 / 响应封装格式

**请求**：`Content-Type: application/json`，body 为 JSON。

**成功响应（HTTP 200）**：

```json
{
  "code": 200,
  "message": "ok",
  "data": { /* 实际数据 */ }
}
```

**错误响应（4xx / 5xx）**：

```json
{
  "code": 404,
  "message": "user_not_found"
}
```

外部平台只需要：
- 看 HTTP status code 决定成功 / 失败
- 成功时读 `data` 字段
- 失败时读 `message` 字段做日志 / 用户提示

### 2.3 URL 编码

`:email` 在 path 里**必须 URL-encode**：

| 原始 email | URL-encoded |
|---|---|
| `alice@example.com` | `alice%40example.com` |
| `bob+tag@mail.cn` | `bob%2Btag%40mail.cn` |
| `张三@测试.com` | `%E5%BC%A0%E4%B8%89%40%E6%B5%8B%E8%AF%95.com` |

各语言用法：

```python
from urllib.parse import quote
path = quote(email, safe='')  # safe='' 强制 encode @
```

```javascript
const path = encodeURIComponent(email);
```

```go
import "net/url"
path := url.PathEscape(email)
```

### 2.4 幂等键策略

调用 `PATCH /users/{email}/balance` 时必须传 `external_op_id`，作为外部平台的**幂等键**。规则：

| 状态 | 二次调用同 `external_op_id` 的行为 |
|---|---|
| 首次提交 | 正常执行，记入 DB |
| 上次 `succeeded` | 返回原结果，`idempotent_replay=true`，**不重复扣加** |
| 上次 `pending` | `409 operation_pending`，外部平台短退避后重查 |
| 上次 `failed` | `409 operation_failed`，**必须换新 `external_op_id` 重试** |

**命名建议**：用前缀区分业务类型，避免不同业务流水撞车：

```
shop-order-99812        # 商城下单
refund-99812            # 商城退款
card-redeem-A1B2C3      # 卡密兑付
manual-adjust-20260527  # 手动对账
```

### 2.5 错误码总览

| HTTP | `message` | 含义 | 处理建议 |
|---|---|---|---|
| 400 | `invalid_request: ...` | 请求体或参数不符规范 | 检查代码 |
| 401 | `unauthorized` | Admin API Key 缺失或错误 | 立即告警 |
| 403 | `forbidden` | 权限不足（通常不会出现） | 检查 key 配置 |
| 404 | `user_not_found` | email 在 sub2api 没找到 | 先调建号接口 |
| 404 | `key_not_found` | key_id 不属于该用户 | 检查 id |
| 409 | `operation_pending` | 同 `external_op_id` 仍在处理 | 退避 + 重查 |
| 409 | `operation_failed` | 同 `external_op_id` 之前失败 | 换新 id 重试 |
| 422 | `insufficient_balance` | set/add 会使余额变负 | 业务逻辑校验 |
| 500 | `internal_error` | 服务端异常 | 告警 + 重试 |

---

## 3. 接口详情

### 3.1 `POST /users` — 创建用户

按 email 创建新用户。如果 email 已存在则幂等返回既有用户信息，**不重发** `api_key` 明文。

#### Request

```
POST /api/v1/openapi/users
x-api-key: <admin_api_key>
Content-Type: application/json
```

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| `email` | string | 是 | — | 邮箱格式校验；自动转小写 |
| `external_user_id` | string | 否 | `""` | 外部平台的用户号，仅做请求标识 |
| `initial_balance` | float | 否 | `0` | 建号即送的余额（USD，单位与平台一致） |
| `key_name` | string | 否 | `"default"` | 自动生成的首个 LLM key 名称 |
| `group_id` | int64 \| null | 否 | `null` | 首个 key 绑定的分组 id |

#### Response

**首次创建（`first_time=true`）**：

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "user_id": 8421,
    "email": "alice@example.com",
    "status": "active",
    "balance": 50.0,
    "api_key": "sk-1234abcd0123...64位hex",
    "api_key_id": 17832,
    "first_time": true
  }
}
```

**email 已存在（幂等重放）**：

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "user_id": 8421,
    "email": "alice@example.com",
    "status": "active",
    "balance": 80.0,
    "first_time": false
  }
}
```

#### 字段说明

| 字段 | 类型 | 何时出现 | 说明 |
|---|---|---|---|
| `user_id` | int64 | 总是 | sub2api 内部用户 id |
| `email` | string | 总是 | 已转小写 |
| `status` | string | 总是 | `active` / `disabled` |
| `balance` | float | 总是 | 当前余额（**已存在的用户返回最新值，不是 `initial_balance`**） |
| `api_key` | string | **仅 `first_time=true`** | LLM API Key 明文，含 `sk-` 前缀 |
| `api_key_id` | int64 | **仅 `first_time=true`** | 对应的 key id，便于后续修改 |
| `first_time` | bool | 总是 | 是否首次创建 |

#### 错误

- `400 invalid_request: ...` — email 格式非法
- `400 invalid_request: ...` — group_id 不存在或当前用户不可绑定

#### curl 示例

```bash
curl -X POST "https://your-domain/api/v1/openapi/users" \
  -H "x-api-key: $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "alice@example.com",
    "external_user_id": "shop-uid-1234",
    "initial_balance": 50.0,
    "key_name": "default"
  }'
```

#### 注意事项

- **`api_key` 仅首次返回一次**，丢失后无法恢复；外部平台必须立即转交给用户（邮件 / 站内信 / 页面展示）并丢弃，**不持久化**
- 内部随机生成的密码不会返回；若用户需要登录 sub2api Web，走"忘记密码"邮件流程
- 重复调用同 email 不会创建多个用户

---

### 3.2 `GET /users/{email}` — 查询用户

#### Request

```
GET /api/v1/openapi/users/alice%40example.com
x-api-key: <admin_api_key>
```

无 query 参数。

#### Response

```json
{
  "code": 200,
  "message": "ok",
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

#### 字段说明

| 字段 | 类型 | 说明 |
|---|---|---|
| `user_id` | int64 | 内部 id |
| `email` | string | 用户邮箱 |
| `status` | string | `active` / `disabled` |
| `role` | string | `user` / `admin` |
| `balance` | float | 当前余额 |
| `total_recharged` | float | 累计充值总额（自动随每次 `add` 增长） |
| `concurrency` | int | 用户级并发上限（LLM 转发） |
| `created_at` | string(ISO8601) | 用户创建时间 |

#### 错误

- `404 user_not_found` — email 不存在

#### curl 示例

```bash
curl "https://your-domain/api/v1/openapi/users/alice%40example.com" \
  -H "x-api-key: $ADMIN_KEY"
```

---

### 3.3 `PATCH /users/{email}/balance` — 调整余额

通过 `external_op_id` 提供幂等保证；支持 `set`（绝对设置）与 `add`（增量加）两种语义。

#### Request

```
PATCH /api/v1/openapi/users/alice%40example.com/balance
x-api-key: <admin_api_key>
Content-Type: application/json
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `external_op_id` | string | 是 | 外部平台幂等键，全局唯一，长度 ≤ 128 |
| `op_type` | string | 是 | `"set"` 或 `"add"` |
| `amount` | float | 是 | 操作金额，必须 ≥ 0 |
| `note` | string | 否 | 审计备注，长度 ≤ 255 |

#### Response

```json
{
  "code": 200,
  "message": "ok",
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

#### 字段说明

| 字段 | 类型 | 说明 |
|---|---|---|
| `operation_id` | int64 | sub2api 内部生成的 op id |
| `balance_before` | float | 操作前余额（事务快照） |
| `balance_after` | float | 操作后余额 |
| `idempotent_replay` | bool | `true` 表示本次是幂等重放，没有真的扣加 |

#### 语义

- **`op_type=add`**：`balance = balance + amount`
- **`op_type=set`**：`balance = amount`（直接覆盖，常用于对账修正）

#### 错误

- `400 invalid_request: ...` — `op_type` 非法 / `amount < 0` / `external_op_id` 为空
- `404 user_not_found` — email 不存在
- `409 operation_pending` — 同 `external_op_id` 仍在处理
- `409 operation_failed` — 同 `external_op_id` 之前失败（必须换新 id 重试）
- `422 insufficient_balance` — `op_type=set` 且 `amount` 会让某些隐含计算变负

#### curl 示例

```bash
# 加余额（首次）
curl -X PATCH "https://your-domain/api/v1/openapi/users/alice%40example.com/balance" \
  -H "x-api-key: $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "external_op_id": "shop-order-99812",
    "op_type": "add",
    "amount": 30.0,
    "note": "order=99812"
  }'

# 同 external_op_id 重放（返回原结果，不重复扣加）
curl -X PATCH "https://your-domain/api/v1/openapi/users/alice%40example.com/balance" \
  -H "x-api-key: $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "external_op_id": "shop-order-99812",
    "op_type": "add",
    "amount": 30.0
  }'
# 响应包含 "idempotent_replay": true
```

#### 注意事项

- 外部平台**必须传 `external_op_id`** 才能保证幂等。无论网络抖动多少次重试，余额最多被加 1 次
- `add` 与 `set` 在 sub2api 内部都会原子加锁（`SELECT ... FOR UPDATE`），高并发安全
- 不支持 `sub`（减余额）。如需扣减，请用 `set` 直接设置到目标值（仅限对账场景使用）

---

### 3.4 `POST /users/{email}/keys` — 创建 LLM Key

代指定用户生成一个新的 LLM API Key（即 `sk-xxx`，用户用它调用 `/v1/messages` 等转发接口）。

#### Request

```
POST /api/v1/openapi/users/alice%40example.com/keys
x-api-key: <admin_api_key>
Content-Type: application/json
```

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| `name` | string | 是 | — | key 名称（用户可见） |
| `group_id` | int64 \| null | 否 | `null` | 绑定分组（影响计费倍率） |
| `custom_key` | string \| null | 否 | `null` | 自定义 key 字符串（≥16 字符；不传则随机生成 `sk-` + 64位hex） |
| `expires_in_days` | int \| null | 否 | `null` | 过期天数（不传则永不过期） |
| `quota` | float \| null | 否 | `0` | key 配额（USD，`0` 表示不限） |
| `rate_limit_5h` | float \| null | 否 | `0` | 5 小时滚动窗口的 USD 上限 |
| `rate_limit_1d` | float \| null | 否 | `0` | 1 天滚动窗口的 USD 上限 |
| `rate_limit_7d` | float \| null | 否 | `0` | 7 天滚动窗口的 USD 上限 |
| `ip_whitelist` | string[] | 否 | `[]` | IP/CIDR 白名单 |
| `ip_blacklist` | string[] | 否 | `[]` | IP/CIDR 黑名单 |

#### Response

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "id": 17833,
    "user_id": 8421,
    "key": "sk-9876fedc...64位hex",
    "name": "for-claude-cli",
    "group_id": null,
    "status": "active",
    "quota": 100.0,
    "quota_used": 0.0,
    "expires_at": "2027-05-27T08:00:00Z",
    "rate_limit_5h": 0.0,
    "rate_limit_1d": 0.0,
    "rate_limit_7d": 0.0,
    "ip_whitelist": [],
    "ip_blacklist": [],
    "created_at": "2026-05-27T08:00:00Z",
    "updated_at": "2026-05-27T08:00:00Z"
  }
}
```

#### 字段说明

| 字段 | 说明 |
|---|---|
| `key` | **明文 sk-xxx，仅本次返回**，后续列表 / 查询接口只返回脱敏前缀 |
| `quota_used` | 累积消费金额（USD） |
| `status` | `active` / `disabled` / `expired` / `quota_exhausted` |

#### 错误

- `404 user_not_found` — email 不存在
- `400 invalid_request: ...` — `custom_key` 太短或字符不合法

#### curl 示例

```bash
curl -X POST "https://your-domain/api/v1/openapi/users/alice%40example.com/keys" \
  -H "x-api-key: $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production",
    "quota": 100.0,
    "expires_in_days": 365,
    "rate_limit_1d": 20.0
  }'
```

---

### 3.5 `GET /users/{email}/keys` — 列出 Keys

返回该用户的所有 key，**不含明文**。

#### Request

```
GET /api/v1/openapi/users/alice%40example.com/keys?status=active
x-api-key: <admin_api_key>
```

| Query | 类型 | 必填 | 说明 |
|---|---|---|---|
| `status` | string | 否 | 过滤：`active` / `disabled` / `expired` / `quota_exhausted` |

#### Response

```json
{
  "code": 200,
  "message": "ok",
  "data": [
    {
      "id": 17832,
      "user_id": 8421,
      "key": "sk-1234abcd******",
      "name": "default",
      "status": "active",
      "quota": 0.0,
      "quota_used": 5.32,
      "rate_limit_5h": 0.0,
      "rate_limit_1d": 0.0,
      "rate_limit_7d": 0.0,
      "expires_at": null,
      "created_at": "2026-05-27T07:00:00Z"
    }
  ]
}
```

`key` 字段为脱敏前缀（前 8 位 + `******`），仅用于用户识别。

#### 错误

- `404 user_not_found`

---

### 3.6 `PATCH /users/{email}/keys/{key_id}` — 修改 Key

#### Request

```
PATCH /api/v1/openapi/users/alice%40example.com/keys/17832
x-api-key: <admin_api_key>
Content-Type: application/json
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `name` | string | 改名（空字符串表示不变） |
| `group_id` | int64 \| null | 改分组 |
| `status` | string | `active` / `inactive`（停用） |
| `ip_whitelist` | string[] | 改白名单（传空数组清空） |
| `ip_blacklist` | string[] | 改黑名单 |
| `quota` | float | 改 quota（`0` = 不限） |
| `expires_at` | string \| null | ISO8601；空字符串清除过期 |
| `reset_quota` | bool | `true` 时把 `quota_used` 清零 |
| `rate_limit_5h` / `rate_limit_1d` / `rate_limit_7d` | float | 改限流 |
| `reset_rate_limit_usage` | bool | `true` 时把滚动窗口用量清零 |

字段都是可选的，**传什么改什么**。

#### Response

与 [3.4](#34-post-usersemailkeys--创建-llm-key) 的 Response 一致，但 `key` 字段是脱敏前缀（修改不会回传明文）。

#### 错误

- `404 user_not_found` / `key_not_found`
- `403 forbidden` — 该 key 不属于此 email 的用户（防越权）

#### curl 示例

```bash
# 提高 quota 并重置已用
curl -X PATCH "https://your-domain/api/v1/openapi/users/alice%40example.com/keys/17832" \
  -H "x-api-key: $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "quota": 200.0,
    "reset_quota": true
  }'

# 停用 key
curl -X PATCH "https://your-domain/api/v1/openapi/users/alice%40example.com/keys/17832" \
  -H "x-api-key: $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{"status": "inactive"}'
```

---

### 3.7 `DELETE /users/{email}/keys/{key_id}` — 删除 Key

#### Request

```
DELETE /api/v1/openapi/users/alice%40example.com/keys/17832
x-api-key: <admin_api_key>
```

#### Response

```json
{
  "code": 200,
  "message": "ok",
  "data": { "deleted": true }
}
```

软删（实际行为依 sub2api 内部实现，可能是 `deleted_at` 标记）。

#### 错误

- `404 user_not_found` / `key_not_found`
- `403 forbidden`

---

### 3.8 `GET /users/{email}/usage` — 消费明细

#### Request

```
GET /api/v1/openapi/users/alice%40example.com/usage?page=1&page_size=50&model=claude-sonnet-4-6
x-api-key: <admin_api_key>
```

| Query | 类型 | 默认 | 最大 | 说明 |
|---|---|---|---|---|
| `page` | int | `1` | — | 页码 |
| `page_size` | int | `50` | `200` | 每页条数 |
| `model` | string | — | — | 按模型过滤 |
| `api_key_id` | int64 | — | — | 按 key id 过滤 |

#### Response

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "items": [
      {
        "id": 9876543,
        "user_id": 8421,
        "api_key_id": 17832,
        "model": "claude-sonnet-4-6",
        "prompt_tokens": 1024,
        "completion_tokens": 256,
        "cache_creation_tokens": 0,
        "cache_read_tokens": 512,
        "total_cost": 0.0125,
        "actual_cost": 0.0125,
        "duration_ms": 1234,
        "created_at": "2026-05-27T08:30:15Z"
      }
    ],
    "pagination": {
      "page": 1,
      "page_size": 50,
      "total": 312
    }
  }
}
```

字段名以实际响应为准（usage_log 表字段，可能含 `account_id`、`group_id` 等额外字段）。

#### 错误

- `404 user_not_found`

---

### 3.9 `GET /users/{email}/usage/stats` — 消费聚合

#### Request

```
GET /api/v1/openapi/users/alice%40example.com/usage/stats?start_at=2026-05-20T00:00:00Z&end_at=2026-05-27T00:00:00Z
x-api-key: <admin_api_key>
```

| Query | 类型 | 默认 | 说明 |
|---|---|---|---|
| `start_at` | string(ISO8601) | 7 天前 | 起始时间（含） |
| `end_at` | string(ISO8601) | now | 结束时间（含） |

**时间窗口最大 90 天**，超过返回 `400 invalid_request: time window exceeds 90 days`。

#### Response

```json
{
  "code": 200,
  "message": "ok",
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

#### 字段说明

| 字段 | 类型 | 说明 |
|---|---|---|
| `total_requests` | int64 | 总请求数 |
| `total_input_tokens` | int64 | 输入 tokens（含 cache_read） |
| `total_output_tokens` | int64 | 输出 tokens |
| `total_cache_tokens` | int64 | 缓存读 / 写 tokens |
| `total_tokens` | int64 | 上面三项之和 |
| `total_cost` | float | 原始 cost（USD） |
| `total_actual_cost` | float | 应用计费倍率后的实际成本 |
| `average_duration_ms` | float | 平均请求耗时 |

#### 错误

- `404 user_not_found`
- `400 invalid_request: invalid start_at: ...`
- `400 invalid_request: time window exceeds 90 days`

---

## 4. 集成场景示例

### 4.1 新用户首次充值

**业务流程**：用户在你的商城注册并下了第一笔订单，付款成功后给 ta 开 sub2api 账号并充值。

```
[外部商城]                                  [sub2api openapi]
    │                                            │
    │  1. POST /openapi/users                    │
    │     {email, initial_balance: 0}            │
    ├───────────────────────────────────────────>│
    │                                            │
    │  2. 200 {first_time:true, api_key:"sk-..."}│
    │<───────────────────────────────────────────┤
    │                                            │
    │  3. (商城存储 user_id + 转交 api_key 给用户)│
    │                                            │
    │  4. PATCH /users/{email}/balance           │
    │     {external_op_id:"shop-order-1",        │
    │      op_type:"add", amount:50}             │
    ├───────────────────────────────────────────>│
    │                                            │
    │  5. 200 {balance_after: 50}                │
    │<───────────────────────────────────────────┤
```

或者**合并为一次调用**（创建用户时直接带 `initial_balance`）：

```
[外部商城]                                  [sub2api openapi]
    │                                            │
    │  POST /openapi/users                       │
    │  {email, initial_balance:50}               │
    ├───────────────────────────────────────────>│
    │                                            │
    │  200 {first_time:true, balance:50,         │
    │       api_key:"sk-...", api_key_id:17832}  │
    │<───────────────────────────────────────────┤
```

**注意**：合并方式不经过 `balance_operations` 幂等表（建号本身用 email unique 保证幂等）；如果外部平台希望两笔分开做账，用第一种。

### 4.2 老用户再次充值

```python
# 用户在商城再付了一笔
def on_payment_success(email, order_id, amount):
    # 直接 add 余额，external_op_id 用订单号确保幂等
    result = client.add_balance(
        email,
        amount=amount,
        external_op_id=f"shop-order-{order_id}",
        note=f"top-up via order {order_id}"
    )
    if result["idempotent_replay"]:
        log.info(f"order {order_id} already credited, balance={result['balance_after']}")
    else:
        log.info(f"order {order_id} credited +{amount}, balance={result['balance_after']}")
```

### 4.3 仪表板展示用户余额与用量

```python
def render_dashboard(email):
    # 基础信息
    info = client.get_user(email)
    balance = info["balance"]
    total_recharged = info["total_recharged"]
    spent = total_recharged - balance  # 估算

    # 用量聚合（最近 30 天）
    from datetime import datetime, timezone, timedelta
    end = datetime.now(timezone.utc)
    start = end - timedelta(days=30)
    stats = client.usage_stats(email,
        start.isoformat().replace("+00:00", "Z"),
        end.isoformat().replace("+00:00", "Z"))

    return {
        "balance": balance,
        "total_recharged": total_recharged,
        "spent_30d": stats["total_actual_cost"],
        "requests_30d": stats["total_requests"],
        "tokens_30d": stats["total_tokens"],
    }
```

### 4.4 幂等重试 / 网络抖动恢复

```python
import time

def add_balance_with_retry(client, email, amount, op_id, max_retries=3):
    """
    网络抖动时安全重试。external_op_id 不变，sub2api 保证最多扣加一次。
    """
    for attempt in range(max_retries):
        try:
            return client.add_balance(email, amount, op_id)
        except requests.HTTPError as e:
            status = e.response.status_code
            body = e.response.json()
            if status == 409 and body["message"] == "operation_pending":
                # 上一笔还在处理，退避后重查
                time.sleep(2 ** attempt)
                continue
            if status == 409 and body["message"] == "operation_failed":
                # 之前失败，必须换 id
                raise RuntimeError(
                    f"op_id {op_id} previously failed, use a new id"
                ) from e
            if status >= 500:
                # 服务端临时错误，退避重试
                time.sleep(2 ** attempt)
                continue
            raise  # 4xx 其它错误不重试
    raise RuntimeError(f"failed after {max_retries} retries")
```

---

## 5. 客户端 SDK 示例

### 5.1 Python

需要 `pip install requests`。

```python
import os
from urllib.parse import quote
import requests

class Sub2APIClient:
    def __init__(self, base_url: str, admin_key: str, timeout: float = 10):
        self.base = base_url.rstrip("/")
        self.timeout = timeout
        self.headers = {
            "x-api-key": admin_key,
            "Content-Type": "application/json",
        }

    def _enc(self, email: str) -> str:
        return quote(email, safe="")

    def _request(self, method, path, **kwargs):
        url = f"{self.base}{path}"
        r = requests.request(method, url, headers=self.headers, timeout=self.timeout, **kwargs)
        r.raise_for_status()
        return r.json()["data"]

    # === 用户 ===

    def create_user(self, email, initial_balance=0, key_name="default",
                    group_id=None, external_user_id=""):
        return self._request("POST", "/users", json={
            "email": email,
            "initial_balance": initial_balance,
            "key_name": key_name,
            "group_id": group_id,
            "external_user_id": external_user_id,
        })

    def get_user(self, email):
        return self._request("GET", f"/users/{self._enc(email)}")

    # === 余额 ===

    def add_balance(self, email, amount, external_op_id, note=""):
        return self._request("PATCH",
            f"/users/{self._enc(email)}/balance",
            json={
                "external_op_id": external_op_id,
                "op_type": "add",
                "amount": amount,
                "note": note,
            })

    def set_balance(self, email, amount, external_op_id, note=""):
        return self._request("PATCH",
            f"/users/{self._enc(email)}/balance",
            json={
                "external_op_id": external_op_id,
                "op_type": "set",
                "amount": amount,
                "note": note,
            })

    # === Keys ===

    def create_key(self, email, name, quota=0, expires_in_days=None,
                   group_id=None, **rate_limits):
        body = {"name": name, "quota": quota, "group_id": group_id}
        if expires_in_days is not None:
            body["expires_in_days"] = expires_in_days
        body.update(rate_limits)
        return self._request("POST", f"/users/{self._enc(email)}/keys", json=body)

    def list_keys(self, email, status=None):
        params = {"status": status} if status else {}
        return self._request("GET", f"/users/{self._enc(email)}/keys", params=params)

    def update_key(self, email, key_id, **fields):
        return self._request("PATCH",
            f"/users/{self._enc(email)}/keys/{key_id}", json=fields)

    def delete_key(self, email, key_id):
        return self._request("DELETE", f"/users/{self._enc(email)}/keys/{key_id}")

    # === 用量 ===

    def list_usage(self, email, page=1, page_size=50, model=None, api_key_id=None):
        params = {"page": page, "page_size": page_size}
        if model:
            params["model"] = model
        if api_key_id:
            params["api_key_id"] = api_key_id
        return self._request("GET", f"/users/{self._enc(email)}/usage", params=params)

    def usage_stats(self, email, start_at=None, end_at=None):
        params = {}
        if start_at:
            params["start_at"] = start_at
        if end_at:
            params["end_at"] = end_at
        return self._request("GET",
            f"/users/{self._enc(email)}/usage/stats", params=params)


# === 用法 ===
client = Sub2APIClient(
    base_url="https://your-domain/api/v1/openapi",
    admin_key=os.environ["SUB2API_ADMIN_KEY"],
)

# 开户
info = client.create_user("alice@example.com", initial_balance=50)
if info["first_time"]:
    send_to_user(info["email"], api_key=info["api_key"])

# 加余额
result = client.add_balance("alice@example.com", 30, "shop-order-99812")

# 查用量
stats = client.usage_stats("alice@example.com",
                            "2026-05-20T00:00:00Z", "2026-05-27T00:00:00Z")
```

### 5.2 Node.js (TypeScript)

需要 `npm i axios`。

```typescript
import axios, { AxiosInstance } from "axios";

interface CreateUserResult {
  user_id: number;
  email: string;
  status: string;
  balance: number;
  api_key?: string;
  api_key_id?: number;
  first_time: boolean;
}

interface AdjustBalanceResult {
  operation_id: number;
  user_id: number;
  email: string;
  op_type: "set" | "add";
  amount: number;
  balance_before: number;
  balance_after: number;
  idempotent_replay: boolean;
}

export class Sub2APIClient {
  private http: AxiosInstance;

  constructor(baseUrl: string, adminKey: string) {
    this.http = axios.create({
      baseURL: baseUrl.replace(/\/$/, ""),
      headers: { "x-api-key": adminKey, "Content-Type": "application/json" },
      timeout: 10_000,
    });
  }

  private encEmail(email: string): string {
    return encodeURIComponent(email);
  }

  async createUser(params: {
    email: string;
    initial_balance?: number;
    key_name?: string;
    group_id?: number | null;
    external_user_id?: string;
  }): Promise<CreateUserResult> {
    const r = await this.http.post("/users", params);
    return r.data.data;
  }

  async getUser(email: string) {
    const r = await this.http.get(`/users/${this.encEmail(email)}`);
    return r.data.data;
  }

  async addBalance(
    email: string,
    amount: number,
    externalOpId: string,
    note = "",
  ): Promise<AdjustBalanceResult> {
    const r = await this.http.patch(
      `/users/${this.encEmail(email)}/balance`,
      { external_op_id: externalOpId, op_type: "add", amount, note },
    );
    return r.data.data;
  }

  async setBalance(
    email: string,
    amount: number,
    externalOpId: string,
    note = "",
  ): Promise<AdjustBalanceResult> {
    const r = await this.http.patch(
      `/users/${this.encEmail(email)}/balance`,
      { external_op_id: externalOpId, op_type: "set", amount, note },
    );
    return r.data.data;
  }

  async createKey(email: string, params: {
    name: string;
    quota?: number;
    expires_in_days?: number;
    group_id?: number | null;
    rate_limit_5h?: number;
    rate_limit_1d?: number;
    rate_limit_7d?: number;
    ip_whitelist?: string[];
    ip_blacklist?: string[];
  }) {
    const r = await this.http.post(
      `/users/${this.encEmail(email)}/keys`, params,
    );
    return r.data.data;
  }

  async listKeys(email: string, status?: string) {
    const r = await this.http.get(`/users/${this.encEmail(email)}/keys`, {
      params: status ? { status } : {},
    });
    return r.data.data;
  }

  async updateKey(email: string, keyId: number, fields: Record<string, any>) {
    const r = await this.http.patch(
      `/users/${this.encEmail(email)}/keys/${keyId}`, fields,
    );
    return r.data.data;
  }

  async deleteKey(email: string, keyId: number) {
    const r = await this.http.delete(
      `/users/${this.encEmail(email)}/keys/${keyId}`,
    );
    return r.data.data;
  }

  async usageStats(email: string, startAt?: string, endAt?: string) {
    const r = await this.http.get(
      `/users/${this.encEmail(email)}/usage/stats`,
      { params: { start_at: startAt, end_at: endAt } },
    );
    return r.data.data;
  }
}

// === 用法 ===
const client = new Sub2APIClient(
  "https://your-domain/api/v1/openapi",
  process.env.SUB2API_ADMIN_KEY!,
);

const info = await client.createUser({
  email: "alice@example.com",
  initial_balance: 50,
});
if (info.first_time && info.api_key) {
  await sendToUser(info.email, info.api_key);
}

const result = await client.addBalance("alice@example.com", 30, "shop-order-99812");
console.log(`balance after = ${result.balance_after}`);
```

### 5.3 Go

```go
package sub2api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	baseURL  string
	adminKey string
	hc       *http.Client
}

func New(baseURL, adminKey string) *Client {
	return &Client{
		baseURL:  baseURL,
		adminKey: adminKey,
		hc:       &http.Client{Timeout: 10 * time.Second},
	}
}

type apiResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", c.adminKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(res.Body)
	var wrap apiResp
	_ = json.Unmarshal(respBody, &wrap)
	if res.StatusCode >= 400 {
		return fmt.Errorf("sub2api %d: %s", res.StatusCode, wrap.Message)
	}
	if out != nil {
		return json.Unmarshal(wrap.Data, out)
	}
	return nil
}

type CreateUserReq struct {
	Email          string  `json:"email"`
	ExternalUserID string  `json:"external_user_id,omitempty"`
	InitialBalance float64 `json:"initial_balance,omitempty"`
	KeyName        string  `json:"key_name,omitempty"`
	GroupID        *int64  `json:"group_id,omitempty"`
}

type CreateUserResp struct {
	UserID    int64   `json:"user_id"`
	Email     string  `json:"email"`
	Status    string  `json:"status"`
	Balance   float64 `json:"balance"`
	APIKey    string  `json:"api_key,omitempty"`
	APIKeyID  int64   `json:"api_key_id,omitempty"`
	FirstTime bool    `json:"first_time"`
}

func (c *Client) CreateUser(req CreateUserReq) (*CreateUserResp, error) {
	var out CreateUserResp
	if err := c.do("POST", "/users", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type AdjustBalanceReq struct {
	ExternalOpID string  `json:"external_op_id"`
	OpType       string  `json:"op_type"`
	Amount       float64 `json:"amount"`
	Note         string  `json:"note,omitempty"`
}

type AdjustBalanceResp struct {
	OperationID      int64   `json:"operation_id"`
	UserID           int64   `json:"user_id"`
	Email            string  `json:"email"`
	OpType           string  `json:"op_type"`
	Amount           float64 `json:"amount"`
	BalanceBefore    float64 `json:"balance_before"`
	BalanceAfter     float64 `json:"balance_after"`
	IdempotentReplay bool    `json:"idempotent_replay"`
}

func (c *Client) AddBalance(email string, amount float64, opID, note string) (*AdjustBalanceResp, error) {
	var out AdjustBalanceResp
	path := fmt.Sprintf("/users/%s/balance", url.PathEscape(email))
	err := c.do("PATCH", path, AdjustBalanceReq{
		ExternalOpID: opID, OpType: "add", Amount: amount, Note: note,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// 其他接口（GetUser / CreateKey / ListKeys / UsageStats 等）按相同模式扩展
```

### 5.4 PHP

```php
<?php
class Sub2APIClient
{
    private string $base;
    private string $key;

    public function __construct(string $baseUrl, string $adminKey)
    {
        $this->base = rtrim($baseUrl, '/');
        $this->key = $adminKey;
    }

    private function request(string $method, string $path, ?array $body = null): array
    {
        $url = $this->base . $path;
        $ch = curl_init($url);
        $headers = ['x-api-key: ' . $this->key, 'Content-Type: application/json'];
        curl_setopt_array($ch, [
            CURLOPT_RETURNTRANSFER => true,
            CURLOPT_CUSTOMREQUEST  => $method,
            CURLOPT_HTTPHEADER     => $headers,
            CURLOPT_TIMEOUT        => 10,
        ]);
        if ($body !== null) {
            curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($body));
        }
        $resp = curl_exec($ch);
        $code = curl_getinfo($ch, CURLINFO_HTTP_CODE);
        curl_close($ch);
        $data = json_decode($resp, true);
        if ($code >= 400) {
            throw new \RuntimeException("sub2api $code: " . ($data['message'] ?? 'unknown'));
        }
        return $data['data'] ?? [];
    }

    public function createUser(string $email, float $initialBalance = 0): array
    {
        return $this->request('POST', '/users', [
            'email'           => $email,
            'initial_balance' => $initialBalance,
        ]);
    }

    public function addBalance(string $email, float $amount, string $opId, string $note = ''): array
    {
        $path = '/users/' . rawurlencode($email) . '/balance';
        return $this->request('PATCH', $path, [
            'external_op_id' => $opId,
            'op_type'        => 'add',
            'amount'         => $amount,
            'note'           => $note,
        ]);
    }

    public function getUser(string $email): array
    {
        return $this->request('GET', '/users/' . rawurlencode($email));
    }

    public function usageStats(string $email, string $startAt, string $endAt): array
    {
        $qs = http_build_query(['start_at' => $startAt, 'end_at' => $endAt]);
        return $this->request('GET',
            '/users/' . rawurlencode($email) . '/usage/stats?' . $qs);
    }
}

// 用法
$client = new Sub2APIClient(
    'https://your-domain/api/v1/openapi',
    getenv('SUB2API_ADMIN_KEY'),
);
$info = $client->createUser('alice@example.com', 50);
if ($info['first_time']) {
    sendApiKeyToUser($info['email'], $info['api_key']);
}
```

---

## 6. 测试与联调建议

### 6.1 本地 / 测试环境

1. **起一个 sub2api 测试实例**：用 docker-compose 启动，配独立的 PostgreSQL + Redis
2. **生成 Admin API Key**：通过 admin 后台或直接调 `POST /api/v1/admin/settings/admin-api-key/regenerate`
3. **用 curl 跑通流程**（参考第 3 节每个接口的 curl 示例）

### 6.2 联调检查清单

- [ ] HTTPS / TLS 证书正常
- [ ] Admin API Key 通过 `x-api-key` header 传递（不要放 query string）
- [ ] email 在 path 里**已 URL-encode**
- [ ] `external_op_id` 不重复（用业务订单号 + 类型前缀）
- [ ] 4xx 错误时读 `message` 字段；5xx 时退避重试
- [ ] `first_time=true` 时立即转交 `api_key` 给用户，**不持久化**
- [ ] 时间字段统一用 ISO 8601 带 `Z` 后缀（UTC）

### 6.3 排错速查

| 现象 | 检查项 |
|---|---|
| `401 unauthorized` | header 写错？key 过期？被管理员轮换了？ |
| `404 user_not_found` | email 拼写、大小写、空格；先建号再操作 |
| `409 operation_pending` 一直不消 | 上一笔卡在数据库锁；检查 sub2api 服务日志 |
| `idempotent_replay=true` 但金额对不上 | 同 `external_op_id` 之前真的提交过；查 sub2api 日志的 op id |
| `balance_after` 比预期少 | 期间用户有 LLM 调用消耗 → 查 `/usage` |
| Stats 时间窗口报错 | start - end 超过 90 天，拆多次查询 |

---

## 7. 安全建议

### 7.1 凭证管理

- Admin API Key **等同 root**，按对待数据库密码的级别保护
- 用 secret manager / 环境变量存放，**不要硬编码进代码或配置文件**
- 至少每季度轮换一次

### 7.2 网络层

- **强制 HTTPS**：sub2api 必须用 TLS（用 Caddy / Nginx 反代或直接配 cert）
- **IP 白名单**：在反代层限制 `/api/v1/openapi/*` 仅接受外部平台服务器 IP
- **不要暴露 `/api/v1/admin/*`**：openapi 路径走 adminAuth，但 admin 后台也用同样鉴权，公网暴露后果严重；admin 后台建议放内网或 VPN 后

### 7.3 数据传递

- `api_key` 明文（`sk-xxx`）只在 `POST /users` 首次返回，**外部平台必须立即转交给用户后丢弃**
- 内部生成的密码不返回；如用户需要 sub2api Web 登录走"忘记密码"流程
- 日志中**脱敏** `x-api-key` header 与响应里的 `api_key` 字段

### 7.4 异常情况

- 周期性扫描 `balance_operations`（如对账 job）核对外部订单与 sub2api 余额变动
- 监控 `409 operation_failed`：意味着某些幂等键有过失败，可能需要人工排查

---

## 8. FAQ

**Q1：用户在 sub2api Web 能登录吗？**
A：能，但需要先走"忘记密码"邮件流程重置密码——M2M 创建时生成的随机密码不会返回。如果你的用户全程不需要进 sub2api Web，他们只用 `sk-xxx` 调上游 LLM 即可。

**Q2：充值能减余额吗？**
A：本接口不支持 `sub`。如需扣减（退款 / 封号清账），用 `op_type=set` 直接设置到目标值。不允许 set 到负数。

**Q3：sub2api 内部余额单位是什么？**
A：USD（美元），精度 `decimal(20, 8)`。外部平台传 `amount` 时按业务约定换算。

**Q4：同时给一个用户加余额会不会丢加？**
A：不会。sub2api 内部用 `SELECT user FOR UPDATE` 行锁 + 数据库事务，并发安全。

**Q5：`balance_operations` 表会无限膨胀吗？**
A：会增长，但每条 < 1KB，年级别 100 万条以内可接受。后续可加 TTL 清理任务（保留 1-2 年）。

**Q6：能不能 Webhook 通知用户余额到账？**
A：本版本未实现。可以由外部平台在调 `PATCH .../balance` 成功后自己 push 通知。

**Q7：能不能批量调用？**
A：本版本未实现批量接口。外部平台并发调用即可（建议并发度 ≤ 10）。

**Q8：sub2api 后端临时不可用怎么办？**
A：5xx 错误时退避重试（推荐指数退避 1s → 2s → 4s → 8s）。`external_op_id` 不变，sub2api 恢复后会幂等处理，**不会重复扣加**。

**Q9：如果同一 email 在两个外部平台对接 sub2api 怎么办？**
A：两个外部平台共享同一个 sub2api 用户。`external_op_id` 用各自的前缀（如 `mall-a-xxx` / `mall-b-xxx`）避免冲突。余额对账时要联合两边数据。

**Q10：能不能查所有用户列表？**
A：本版本只支持按 email 单查。批量列表请走 sub2api admin 后台 / admin API（`/api/v1/admin/users`）。

---

**版本**：v1.0
**最后更新**：2026-05-27
**反馈联系**：sub2api 项目仓库 Issue
