# Sub2API OpenAPI 平台数据集成接口 — 设计文档

| 元数据 | 值 |
|---|---|
| 日期 | 2026-05-27 |
| 状态 | Draft, awaiting user review |
| 作者 | brainstorming session |
| 适用范围 | sub2api 后端新增一组对外数据集成接口，供"平台到平台"调用 |

---

## 1. 背景与目标

### 1.1 现状

Sub2API 当前已有：

- **JWT 鉴权**：用户登录拿 JWT，调用 `/api/v1/user/*`、`/api/v1/keys/*` 等自助接口
- **API Key 鉴权**：用户生成 `sk-xxx` key 调用 `/v1/messages` 等 LLM 转发接口
- **Admin API Key 鉴权**：管理员后台 `x-api-key` header，调用 `/api/v1/admin/*`
- **内置支付**：EasyPay / 支付宝 / 微信 / Stripe 自助充值闭环

### 1.2 目标

为"另一个外部平台"提供一套**纯数据集成接口**，让外部平台可以：

1. **创建 sub2api 用户**（按 email；存在则幂等返回）
2. **查询用户基础信息和余额**
3. **调整用户余额**（直接 set 或 add；带幂等键防止重复加扣）
4. **代用户管理 LLM API Key**（生成 / 列出 / 改 / 删）
5. **代用户查询消费明细与聚合**（复用 `usage_log`）

### 1.3 非目标

- **不**做用户自助 PAT（Personal Access Token）体系。最终用户**不直接调用 sub2api**，只通过外部平台间接获取数据。
- **不**做密卡兑付。密卡发行/兑付完全在外部平台处理，sub2api 只接受"加余额"动作。
- **不**做 magic-link 免密登录。用户不进 sub2api Web 前端。
- **不**改造现有 LLM API Key 鉴权 / 计费 / 转发链路。

---

## 2. 架构总览

### 2.1 路由

新增一个独立命名空间 `/api/v1/openapi/`，与 `/api/v1/admin/`、`/api/v1/user/` 平级。

```
/api/v1/openapi/         <-- adminAuth() 中间件
├── POST   /users                            # 创建用户
├── GET    /users/:email                     # 查用户
├── PATCH  /users/:email/balance             # 调余额（幂等）
│
├── POST   /users/:email/keys                # 生成 LLM API Key
├── GET    /users/:email/keys                # 列 keys
├── PATCH  /users/:email/keys/:key_id        # 改 key
├── DELETE /users/:email/keys/:key_id        # 删 key
│
├── GET    /users/:email/usage               # 消费明细（分页）
└── GET    /users/:email/usage/stats         # 消费聚合
```

email 在 path 里走 URL-encode（`alice@example.com` → `alice%40example.com`）。

### 2.2 鉴权

**直接复用现有 `adminAuth()` 中间件**：

- Header：`x-api-key: <admin_api_key>`
- key 存于 settings 表，由管理员后台维护
- 不新增中间件、不新增 token 表

风险点：admin api key 同时拥有"调 admin 后台"与"调 openapi"两份权限，**不能独立轮换**。若未来需要拆分，再补 platform_tokens 表（本期不做）。

### 2.3 数据模型

#### 新增表（1 张）：`balance_operations`

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | int64 | PK |
| `external_op_id` | string(128) | **外部平台传入的操作号**，幂等键 |
| `user_id` | int64 | 目标用户 |
| `op_type` | enum | `set` / `add` |
| `amount` | decimal(20,8) | 操作金额，与 `users.balance` 同单位 |
| `balance_before` | decimal(20,8) | 执行前余额（事务内快照） |
| `balance_after` | decimal(20,8) | 执行后余额 |
| `status` | enum | `pending` / `succeeded` / `failed` |
| `failure_reason` | text nullable | |
| `note` | string(255) nullable | 外部平台传入备注，如 "redeem-card-X1234" |
| `request_payload` | jsonb | 完整入参，审计用 |
| `created_at` / `updated_at` | timestamptz | |

**索引**：
- `external_op_id` UNIQUE（防重复加扣）
- `user_id, created_at` 复合（查某用户历史）

#### 现有表零改动

- `users`：不加字段
- `api_keys`：不加字段
- `usage_log`：不加字段
- `redeem_codes`：不涉及

---

## 3. 接口契约

### 3.1 `POST /api/v1/openapi/users` — 创建用户

**请求**：

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
| `external_user_id` | 否 | — | 外部平台的用户号，仅用于审计备注 |
| `initial_balance` | 否 | `0` | 建号即送的余额 |
| `key_name` | 否 | `"default"` | 自动生成的首个 LLM key 名称 |
| `group_id` | 否 | `null` | 首个 key 绑定的分组 id |

**响应（首次成功）**：

```json
{
  "user_id": 8421,
  "email": "alice@example.com",
  "status": "active",
  "balance": 50.0,
  "api_key": "sk-1234abcd...64hex",
  "api_key_id": 17832,
  "first_time": true
}
```

**响应（email 已存在 — 幂等重放）**：

```json
{
  "user_id": 8421,
  "email": "alice@example.com",
  "status": "active",
  "balance": 80.0,
  "first_time": false
}
```

幂等重放**不重发** `api_key` 明文（首次响应即丢失，无法恢复）。

**事务步骤**：

```
[BEGIN TX]
1. SELECT user WHERE email=? FOR UPDATE
   ├── 存在 → 返回 user_id + 当前 balance，first_time=false，跳到 COMMIT
   └── 不存在 ↓
2. 生成 32 字节随机密码 → bcrypt → password_hash
   （密码永不外露；用户若需登录 Web，走"忘记密码"邮件重置）
3. INSERT users (email, password_hash, role='user', status='active', balance=initial_balance)
4. 复用 api_key_service.CreateAPIKey(user_id, name=key_name, group_id)
   → 拿到 sk-xxx 明文
[COMMIT]
```

**错误**：
- email 格式非法 → 400
- group_id 不存在或不可用 → 400

### 3.2 `GET /api/v1/openapi/users/:email` — 查询用户

**响应**：

```json
{
  "user_id": 8421,
  "email": "alice@example.com",
  "status": "active",
  "role": "user",
  "balance": 80.0,
  "total_recharged": 80.0,
  "concurrency": 5,
  "groups": ["default"],
  "created_at": "2026-05-27T07:00:00Z"
}
```

**错误**：email 不存在 → 404

### 3.3 `PATCH /api/v1/openapi/users/:email/balance` — 调整余额

**请求**：

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
| `external_op_id` | 是 | 外部平台幂等键 |
| `op_type` | 是 | `set` 或 `add` |
| `amount` | 是 | 操作金额，必须 ≥ 0 |
| `note` | 否 | 审计备注 |

**响应**：

```json
{
  "operation_id": 5566,
  "user_id": 8421,
  "email": "alice@example.com",
  "op_type": "add",
  "amount": 30.0,
  "balance_before": 50.0,
  "balance_after": 80.0,
  "idempotent_replay": false
}
```

**事务步骤**：

```
[BEGIN TX]
1. INSERT balance_operations (external_op_id, user_id, op_type, amount, status='pending')
   ON CONFLICT (external_op_id) DO NOTHING
   ├── 成功 → 拿到 op_id，走步骤 2
   └── 冲突 → 读旧记录：
        - succeeded → 直接返回旧 balance_before/after，idempotent_replay=true
        - pending   → 409 operation_pending（让外部短退避后重查）
        - failed    → 409 operation_failed，**外部平台必须用新的 external_op_id 重试**
                      （失败行永久保留作审计；不复用 external_op_id 可避免对账歧义）
2. SELECT users.balance WHERE id=user_id FOR UPDATE → balance_before
3. 计算 balance_after:
     - op_type='set' → balance_after = amount
     - op_type='add' → balance_after = balance_before + amount
   校验：balance_after ≥ 0
4. UPDATE users SET balance=balance_after WHERE id=user_id
5. UPDATE balance_operations SET status='succeeded', balance_before, balance_after WHERE id=op_id
[COMMIT]

异常路径：UPDATE balance_operations SET status='failed', failure_reason 后回滚事务
```

**关键设计**：
- `FOR UPDATE` 锁住用户行，防止并发 add 丢加
- `external_op_id` 单字段 UNIQUE（不带 platform_token_id，因为复用 admin key 时无平台维度区分）
- `op_type=set` 主要用于对账修正，不限频次

**错误**：
- email 不存在 → 404
- balance_after < 0 → 422
- amount < 0 → 400

### 3.4 `POST /api/v1/openapi/users/:email/keys` — 生成 LLM Key

**请求**：与现有 `POST /api/v1/keys` 的 `CreateAPIKeyRequest` 完全一致：

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

**响应**：与现有接口一致，**含 sk-xxx 明文（仅首次返回）**。

**实现**：handler 解析 `:email` → user_id → 直接调 `api_key_service.CreateAPIKey()`。

### 3.5 `GET /api/v1/openapi/users/:email/keys` — 列 key

**Query**：`page`、`page_size`、`status`（active/disabled）、`group_id`

**响应**：现有 ApiKey List 结构，**不含明文 key**，含 `token_prefix`。

### 3.6 `PATCH /api/v1/openapi/users/:email/keys/:key_id` — 改 key

复用 `api_key_service.Update()`。handler 校验 `key.user_id == 查到的 user.id` 防越权。

### 3.7 `DELETE /api/v1/openapi/users/:email/keys/:key_id` — 删 key

复用 `api_key_service.Delete()`。同样做归属校验。

### 3.8 `GET /api/v1/openapi/users/:email/usage` — 消费明细

**Query**：
- `start_at` / `end_at`（ISO8601，默认最近 7 天）
- `api_key_id`（可选）
- `model`（可选）
- `page` / `page_size`（默认 1 / 50，最大 page_size = 200）

**响应**：

```json
{
  "items": [
    {
      "id": 12345,
      "created_at": "2026-05-27T08:00:00Z",
      "api_key_id": 17832,
      "api_key_prefix": "sk-abcd1234",
      "model": "claude-sonnet-4-6",
      "prompt_tokens": 1024,
      "completion_tokens": 256,
      "total_cost": 0.0125,
      "actual_cost": 0.0125,
      "group_id": 1
    }
  ],
  "pagination": {"page": 1, "page_size": 50, "total": 312}
}
```

### 3.9 `GET /api/v1/openapi/users/:email/usage/stats` — 消费聚合

**Query**：
- `start_at` / `end_at`（默认最近 7 天，最大 90 天）
- `group_by`：`day` / `model` / `api_key`（默认 `day`）

**响应**：

```json
{
  "start_at": "2026-05-20T00:00:00Z",
  "end_at": "2026-05-27T00:00:00Z",
  "group_by": "day",
  "buckets": [
    {"key": "2026-05-20", "calls": 122, "tokens": 312000, "cost": 1.85}
  ],
  "total": {"calls": 812, "tokens": 2100000, "cost": 12.65}
}
```

聚合 SQL：`SELECT date_trunc('day', created_at), COUNT(*), SUM(tokens), SUM(actual_cost) FROM usage_log WHERE user_id=? AND created_at BETWEEN ? AND ? GROUP BY 1`。

---

## 4. 错误响应

统一沿用现有 `middleware.RespondError`：

```json
{
  "error": "user_not_found",
  "message": "user with email alice@example.com not found"
}
```

错误码表（与 HTTP status 配套）：

| HTTP | error | 说明 |
|---|---|---|
| 400 | `invalid_request` | 入参校验失败 |
| 401 | `unauthorized` | admin api key 缺失或错误 |
| 404 | `user_not_found` / `key_not_found` | |
| 409 | `operation_pending` | 同 external_op_id 仍在处理 |
| 409 | `operation_failed` | 同 external_op_id 之前失败，必须换新 id 重试 |
| 422 | `insufficient_balance` | set/add 会使余额负数 |
| 500 | `internal_error` | 兜底 |

---

## 5. 文件结构

新增文件：

```
backend/
├── ent/schema/
│   └── balance_operation.go              # ent schema
├── internal/repository/
│   └── balance_operation_repo.go         # repo 层
├── internal/service/
│   └── openapi_service.go                # service 层（聚合 user/balance/key/usage 操作）
├── internal/handler/
│   └── openapi_handler.go                # gin handlers
└── internal/server/routes/
    └── openapi.go                        # 路由注册（套 adminAuth）
```

修改文件：

```
backend/internal/server/router.go         # 注册新路由组
```

无现有逻辑改动。

---

## 6. 兼容性 / 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| Admin API Key 权限过大 | 调 openapi 与调 admin 后台共用一个 key，泄漏后影响面广 | 部署时 IP 白名单约束；记录 `last_used_ip` 做异常告警；后续可加 platform_tokens 表拆权 |
| `external_op_id` 全局唯一 | 不同业务方传相同 `external_op_id` 会冲突 | 文档约束：建议外部平台前缀化（`shop-`、`refund-` 等） |
| 自动生成密码用户无法登录 Web | 用户若想自助看后台需先走"忘记密码" | 文档说明；本期可接受（用户不进前端） |
| 删除 / 禁用 key 时仍有未结算请求 | 现有 LLM 转发链路已处理 | 复用现有逻辑，本设计零改动 |
| 余额并发 add 丢加 | 高并发场景 balance_before 读旧 | `FOR UPDATE` 行锁 + 事务覆盖 |

---

## 7. 测试策略

| 层 | 内容 |
|---|---|
| 单元测试 | balance op 计算、幂等表冲突分支、user_ref → user_id 解析 |
| 集成测试 | 全量 9 个 endpoint 的成功/失败路径、并发 add 一致性、email 不存在 / 已存在分支、key 越权访问拦截 |
| 手测 | curl 脚本覆盖创建用户 + 加余额 + 生成 key + 查用量的完整链路 |

测试标签沿用项目惯例：`-tags=unit` / `-tags=integration`。

---

## 8. 部署 / 迁移

1. Ent 生成：`go generate ./ent`
2. DB migration：自动生成（Ent automigrate 或 atlas，依项目惯例）；只新增 `balance_operations` 表，无破坏性变更
3. 配置：无新增 env var（复用现有 admin api key）
4. 文档：补一份 `docs/OPENAPI_PLATFORM.md` 用户向集成指南（实现阶段一并产出）

---

## 9. 后续可能扩展（非本期）

- 独立的 `platform_tokens` 表，拆 admin key 与 openapi key 的权限
- 多个 platform token 独立轮换、IP 白名单、速率限制
- C 端用户 PAT 自助接口
- 余额变更 webhook（推送给外部平台对账）
- 卡密兑付接口
