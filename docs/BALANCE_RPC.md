# 余额 RPC 接入文档（Balance Ledger RPC）

面向「扣费 app」接入方：通过 tRPC-Go 在独立端口调用余额账本服务，做余额扣费 / 部分退费 / 查询。

## 1. 概览

- **传输**：tRPC-Go（protocol=`trpc`，network=`tcp`），监听端口独立于 HTTP API（`server.port`）。
- **服务名**：`sub2api.balance.v1.BalanceLedger`
- **方法**：`Deduct` / `Refund` / `GetBalance`
- **proto**：`backend/internal/rpc/balancepb/balance.proto`
- **金额**：一律用**十进制字符串**传输（如 `"12.34"`），服务端按 `decimal(20,8)` 处理，避免二进制浮点误差。
- **边界**：本服务只动 `users.balance`，不触碰 apikey 配额 / 限流 / 账号配额 / 订阅用量（那些由网关计费链路负责）。接入方只能冲销**自己**经本 RPC 产生的扣费。

## 2. 启用与配置

`config.yaml`：

```yaml
balance_rpc:
  enabled: true       # 关闭时不监听第二端口，行为等价现状
  host: "0.0.0.0"
  port: 9100          # 必须 != server.port
  encryption_key: ""  # 32 字节（64 hex 字符），接入方 token 的本地加解密密钥
```

> 这是动钱接口。生产环境务必把该端口限制在内网 / 服务网格内，并叠加网络隔离（可选 mTLS）。
> `encryption_key` 是签发 / 校验所有接入方 token 的本地密钥，独立于 TOTP 等其它密钥；
> 轮换它会一次性作废所有已签发的 token。

## 3. 接入方身份（鉴权）

鉴权采用**无状态 token**：接入方的凭据 `token` = 本地密钥对其 `app_id` 的 **AES-256-GCM 密文**。
服务端每次调用**解密 token**：解密成功（GCM 认证标签校验通过）即证明该 token 由本方签发，
再校验解密出的 `app_id` 对应 app 存在且未停用。**数据库不存储任何 token 或 hash**。

每个接入方是一个「扣费 app」，由管理员在后台创建：

| 操作 | 接口 |
|------|------|
| 列表 | `GET  /api/admin/billing-apps` |
| 创建 | `POST /api/admin/billing-apps`  body `{"app_name":"xxx"}` |
| 费用统计 | `GET  /api/admin/billing-apps/:app_id/stats`（累计扣费/退费/净扣费/笔数） |
| 启停 | `PATCH /api/admin/billing-apps/:app_id/enabled` body `{"enabled":false}` |
| 刷新 token | `POST /api/admin/billing-apps/:app_id/refresh-token`（旧 token 立即失效，返回新 token） |
| 删除 | `DELETE /api/admin/billing-apps/:app_id`（token 立即失效，历史流水保留） |

创建响应包含一次性 `token`（**仅此一次返回**，库里不存）：

```json
{ "id": 1, "app_id": "bapp_xxxx", "app_name": "xxx", "enabled": true, "token": "base64 密文，请妥善保存" }
```

调用 RPC 时通过 **tRPC metadata** 携带 token：

- `app-token`: 创建时拿到的 token

鉴权失败（token 无法解密 / app 不存在 / app 已停用）统一返回 `RetServerAuthFail`，不区分原因。
停用通过本地缓存（短 TTL + 启停时主动失效）生效，热路径不必每请求查 DB。

Go 客户端示例（per-call metadata 携带 token）：

```go
import (
    "trpc.group/trpc-go/trpc-go/client"
    pb "github.com/Wei-Shaw/sub2api/internal/rpc/balancepb"
)

proxy := pb.NewBalanceLedgerClientProxy(
    client.WithTarget("ip://127.0.0.1:9100"),
)
rsp, err := proxy.Deduct(ctx, &pb.DeductRequest{
    UserId:      1,
    RequestId:   "order-123",
    Amount:      "12.34",
    Description: "purchase",
},
    client.WithMetaData("app-token", []byte("创建时拿到的 token")),
)
```

## 4. 方法语义

### Deduct（扣费）

| 字段 | 说明 |
|------|------|
| `user_id` | 目标用户 |
| `request_id` | **幂等键**：同一 app 相同 request_id 只扣一次 |
| `amount` | 扣费金额（十进制字符串，> 0） |
| `description` | **必填**，扣费原因 |
| `extra` | 可选，jsonb 文本，接入方自存数据 |

- **不透支**：余额不足直接拒绝（`INSUFFICIENT_BALANCE`），不会扣成负数。
- 幂等重放返回 `applied=false` + 首次的 `balance_after`。

### Refund（退费，支持部分退）

| 字段 | 说明 |
|------|------|
| `refund_request_id` | **本笔退款幂等键**（与原扣 request_id 不同） |
| `original_request_id` | 被冲销的原扣 `request_id` |
| `amount` | 本次退费金额（可小于原扣，支持多次部分退） |
| `description` | **必填**，退费原因 |
| `extra` | 可选 |

- **凭原流水冲销**：只能退本 app 自己扣过的款；原扣不存在 → `ORIGINAL_DEDUCT_NOT_FOUND`。
- **累计不超额**：同一原扣的累计已退 ≤ 原扣金额，否则 `OVER_REFUND`。
- 响应 `refunded_total` 为该原扣的累计已退。

### GetBalance（查询余额）

入参 `user_id`，返回 `balance`（十进制字符串，缓存优先、未命中回源 DB）。

## 5. 幂等键约定

- **每次 Deduct** 自带唯一 `request_id`；网络超时重试用**同一个** request_id，保证至多扣一次。
- **每次 Refund** 自带唯一 `refund_request_id`；重试用同一个，保证至多退一次。部分退是累加，必须靠 `refund_request_id` 去重——不能只靠「原扣已退」判断。

## 6. 错误码

| 场景 | tRPC code / reason |
|------|------|
| 参数非法（amount≤0 / description 空 / 金额格式错） | `LEDGER_INVALID_AMOUNT` / `LEDGER_DESCRIPTION_REQUIRED`（400） |
| 鉴权失败 / app 停用 | `RetServerAuthFail` |
| 用户不存在 / 原扣流水不存在 | `USER_NOT_FOUND`（404）/ `ORIGINAL_DEDUCT_NOT_FOUND`（404） |
| 余额不足 | `INSUFFICIENT_BALANCE`（409） |
| 累计退款超原扣 | `OVER_REFUND`（409） |
| 同 request_id 参数不一致 | `LEDGER_REQUEST_CONFLICT`（409） |

## 7. 审计

每笔扣 / 退在 `balance_ledger` 表落一行永久流水（含 `app_id` / `description` / `extra` / 余额快照），不被 TTL 清理，可用于对账。
