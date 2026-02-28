# Sub2API Admin API: Payment Integration / 支付集成接口文档

## 中文

### 概述

本文档描述外部支付系统（例如 sub2apipay）对接 Sub2API 时的最小 Admin API 集合，用于完成充值发放与对账。

### 基础地址

- 生产环境：`https://<your-domain>`
- Beta 环境：`http://<your-server-ip>:8084`

### 认证方式

以下接口均建议使用：

- 请求头：`x-api-key: admin-<64hex>`（服务间调用推荐）
- 请求头：`Content-Type: application/json`

说明：管理员 JWT 也可访问 admin 路由，但机器对机器调用建议使用 Admin API Key。

### 1) 一步完成：创建兑换码并兑换

`POST /api/v1/admin/redeem-codes/create-and-redeem`

用途：

- 原子化完成“创建固定兑换码 + 兑换给指定用户”。
- 常用于支付回调成功后的自动充值。

必需请求头：

- `x-api-key`
- `Idempotency-Key`

请求体：

```json
{
  "code": "s2p_REDACTED",
  "type": "balance",
  "value": 100.0,
  "user_id": 123,
  "notes": "sub2apipay order: REDACTED"
REDACTED
```

规则：

- `code`：外部订单映射的确定性兑换码。
- `type`：当前推荐使用 `balance`。
- `value`：必须大于 0。
- `user_id`：目标用户 ID。

幂等语义：

- 同一 `code` 且 `used_by` 一致：返回 `200`（幂等回放）。
- 同一 `code` 但 `used_by` 不一致：返回 `409`（冲突）。
- 缺少 `Idempotency-Key`：返回 `400`（`IDEMPOTENCY_KEY_REQUIRED`）。

示例：

```bash
curl -X POST "${BASEREDACTED/api/v1/admin/redeem-codes/create-and-redeem" \
  -H "x-api-key: ${KEYREDACTED" \
  -H "Idempotency-Key: pay-REDACTED-success" \
  -H "Content-Type: application/json" \
  -d '{
    "code":"s2p_REDACTED",
    "type":"balance",
    "value":100.00,
    "user_id":123,
    "notes":"sub2apipay order: REDACTED"
  REDACTED'
```

### 2) 查询用户（可选前置检查）

`GET /api/v1/admin/users/:id`

用途：

- 支付成功后充值前，确认目标用户是否存在。

示例：

```bash
curl -s "${BASEREDACTED/api/v1/admin/users/123" \
  -H "x-api-key: ${KEYREDACTED"
```

### 3) 余额调整（已存在接口）

`POST /api/v1/admin/users/:id/balance`

用途：

- 复用现有管理员接口做人工纠偏。
- 支持 `set`、`add`、`subtract`。

示例（扣减）：

```json
{
  "balance": 100.0,
  "operation": "subtract",
  "notes": "manual correction"
REDACTED
```

```bash
curl -X POST "${BASEREDACTED/api/v1/admin/users/123/balance" \
  -H "x-api-key: ${KEYREDACTED" \
  -H "Idempotency-Key: balance-subtract-REDACTED" \
  -H "Content-Type: application/json" \
  -d '{
    "balance":100.00,
    "operation":"subtract",
    "notes":"manual correction"
  REDACTED'
```

### 4) 购买页跳转 URL Query 参数（iframe 与新窗口统一）

Sub2API 前端在打开 `purchase_subscription_url` 时，会给 iframe 和“新窗口打开”统一追加 query 参数，确保外部支付页拿到一致上下文。

追加参数：

- `user_id`：当前登录用户 ID
- `token`：当前登录 JWT token
- `theme`：当前主题（`light` / `dark`）
- `ui_mode`：当前 UI 模式（固定 `embedded`）

示例：

```text
https://pay.example.com/pay?user_id=123&token=<jwt>&theme=light&ui_mode=embedded
```

### 5) 失败处理建议

- 支付状态与充值状态分开落库。
- 收到并验证支付回调后，立即标记“支付成功”。
- 支付成功但充值失败的订单应允许后续重试。
- 重试时继续使用同一 `code`，并使用新的 `Idempotency-Key`。

### 6) `doc_url` 配置建议

Sub2API 已支持系统设置中的 `doc_url` 字段。

推荐配置：

- 查看链接：`https://github.com/Wei-Shaw/sub2api/blob/main/ADMIN_PAYMENT_INTEGRATION_API.md`
- 下载链接：`https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/ADMIN_PAYMENT_INTEGRATION_API.md`

---

## English

### Overview

This document defines the minimum Admin API surface for integrating external payment systems (for example, sub2apipay) with Sub2API for recharge fulfillment and reconciliation.

### Base URL

- Production: `https://<your-domain>`
- Beta: `http://<your-server-ip>:8084`

### Authentication

Recommended headers:

- `x-api-key: admin-<64hex>` (recommended for server-to-server calls)
- `Content-Type: application/json`

Note: Admin JWT is also accepted by admin routes, but Admin API key is recommended for machine integrations.

### 1) One-step Create + Redeem

`POST /api/v1/admin/redeem-codes/create-and-redeem`

Purpose:

- Atomically create a deterministic redeem code and redeem it to the target user.
- Typical usage: called right after payment callback success.

Required headers:

- `x-api-key`
- `Idempotency-Key`

Request body:

```json
{
  "code": "s2p_REDACTED",
  "type": "balance",
  "value": 100.0,
  "user_id": 123,
  "notes": "sub2apipay order: REDACTED"
REDACTED
```

Rules:

- `code`: deterministic code mapped from external order id.
- `type`: `balance` is the recommended type.
- `value`: must be greater than 0.
- `user_id`: target user id.

Idempotency behavior:

- Same `code` and same `used_by`: `200` (idempotent replay).
- Same `code` and different `used_by`: `409` (conflict).
- Missing `Idempotency-Key`: `400` (`IDEMPOTENCY_KEY_REQUIRED`).

Example:

```bash
curl -X POST "${BASEREDACTED/api/v1/admin/redeem-codes/create-and-redeem" \
  -H "x-api-key: ${KEYREDACTED" \
  -H "Idempotency-Key: pay-REDACTED-success" \
  -H "Content-Type: application/json" \
  -d '{
    "code":"s2p_REDACTED",
    "type":"balance",
    "value":100.00,
    "user_id":123,
    "notes":"sub2apipay order: REDACTED"
  REDACTED'
```

### 2) Query User (Optional Pre-check)

`GET /api/v1/admin/users/:id`

Purpose:

- Verify target user existence before final recharge/retry.

Example:

```bash
curl -s "${BASEREDACTED/api/v1/admin/users/123" \
  -H "x-api-key: ${KEYREDACTED"
```

### 3) Balance Adjustment (Existing API)

`POST /api/v1/admin/users/:id/balance`

Purpose:

- Reuse existing admin endpoint for manual reconciliation.
- Supports `set`, `add`, `subtract`.

Request body example (`subtract`):

```json
{
  "balance": 100.0,
  "operation": "subtract",
  "notes": "manual correction"
REDACTED
```

Example:

```bash
curl -X POST "${BASEREDACTED/api/v1/admin/users/123/balance" \
  -H "x-api-key: ${KEYREDACTED" \
  -H "Idempotency-Key: balance-subtract-REDACTED" \
  -H "Content-Type: application/json" \
  -d '{
    "balance":100.00,
    "operation":"subtract",
    "notes":"manual correction"
  REDACTED'
```

### 4) Purchase URL Query Parameters (Iframe + New Tab)

When Sub2API frontend opens `purchase_subscription_url`, it appends the same query parameters for both iframe and “open in new tab” to keep context consistent.

Appended parameters:

- `user_id`: current logged-in user id
- `token`: current logged-in JWT token
- `theme`: current theme (`light` / `dark`)
- `ui_mode`: UI mode (fixed `embedded`)

Example:

```text
https://pay.example.com/pay?user_id=123&token=<jwt>&theme=light&ui_mode=embedded
```

### 5) Failure Handling Recommendations

- Store payment state and recharge state separately.
- Mark payment success immediately after callback verification.
- Keep orders retryable when payment succeeded but recharge failed.
- Reuse the same deterministic `code` and a new `Idempotency-Key` when retrying.

### 6) Suggested `doc_url` Value

Sub2API already supports `doc_url` in system settings.

Recommended values:

- View URL: `https://github.com/Wei-Shaw/sub2api/blob/main/ADMIN_PAYMENT_INTEGRATION_API.md`
- Download URL: `https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/ADMIN_PAYMENT_INTEGRATION_API.md`
