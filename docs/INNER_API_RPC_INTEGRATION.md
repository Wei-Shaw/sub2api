# 内部 API RPC 接入文档（Inner API RPC）

面向**内部 API app 接入方**。平台运维/部署视角见 `INNER_API_RPC.md`。

## 1. 这是什么

一个通过 **tRPC-Go** 暴露的内部服务，提供余额账本和素材库能力。每个 app 只能调用被授权的接口：

| 方法 | 作用 |
|------|------|
| `Deduct` | 从某用户余额扣一笔钱（不透支、幂等） |
| `Refund` | 凭原扣流水退款（支持部分退、幂等） |
| `GetBalance` | 查某用户当前余额 |
| `ListMaterials` / `GetMaterial` | 查询素材列表或单个素材 |
| `UploadMaterial` / `AddMaterialByUrl` | 上传素材，或把已有 COS URL 加入素材库 |
| `RenameMaterial` | 修改素材展示名称 |
| `DeleteMaterial` / `BatchDeleteMaterials` | 单个或批量删除素材 |

权限在创建或后台编辑 app 时授予，未授予的方法会返回统一的 `RetServerAuthFail`：

| 权限 | 可调用方法 |
|------|------------|
| `balance:write` | `Deduct`, `Refund` |
| `balance:read` | `GetBalance` |
| `materials:read` | `ListMaterials`, `GetMaterial` |
| `materials:write` | `UploadMaterial`, `AddMaterialByUrl`, `RenameMaterial`, `DeleteMaterial`, `BatchDeleteMaterials` |

- 服务名：`sub2api.inner.v1.InnerAPI`
- 监听：**独立端口**（由平台方在 `inner_api_rpc.port` 配置，例如 `9100`），与 HTTP API 不同端口
- 协议：tRPC（`network=tcp`, `protocol=trpc`）
- 金额：**一律十进制字符串**（如 `"12.34"`），不要用浮点

## 2. 接入前你会拿到什么

找平台管理员在后台「内部 API App」页面给你创建一个 app，你会拿到：

- `app_id`：形如 `iapp_xxxxxxxx`（标识你这个接入方）
- `token`：一段 base64 字符串，**只展示一次**，请立即保存到你服务的密钥管理里

> token 就是你的凭据（平台用本地密钥加密 `app_id` + 版本号得到的密文）。平台**不存**这个 token，丢了或被泄露请让管理员「刷新 token」（旧 token 立即作废）。

## 3. 鉴权方式

每次调用在 **tRPC metadata** 里带上 token，键名固定为 `app-token`：

```
metadata: { "app-token": "<你拿到的 token>" }
```

服务端解密成功、token 版本是最新、且你的 app 未被停用 → 通过；否则返回鉴权失败。
管理员「刷新 token」后，**旧 token 立即失效**，请换用新 token。

## 4. proto 契约

```proto
syntax = "proto3";
package sub2api.inner.v1;

service InnerAPI {
  rpc Deduct(DeductRequest) returns (DeductResponse);
  rpc Refund(RefundRequest) returns (RefundResponse);
  rpc GetBalance(GetBalanceRequest) returns (GetBalanceResponse);
  rpc ListMaterials(ListMaterialsRequest) returns (ListMaterialsResponse);
  rpc GetMaterial(GetMaterialRequest) returns (Material);
  rpc UploadMaterial(UploadMaterialRequest) returns (UploadMaterialResponse);
  rpc AddMaterialByUrl(AddMaterialByUrlRequest) returns (AddMaterialByUrlResponse);
  rpc RenameMaterial(RenameMaterialRequest) returns (Material);
  rpc DeleteMaterial(DeleteMaterialRequest) returns (DeleteMaterialResponse);
  rpc BatchDeleteMaterials(BatchDeleteMaterialsRequest) returns (BatchDeleteMaterialsResponse);
}

message DeductRequest {
  string account_id  = 1;  // 对外账户标识，不是 users.id
  string request_id  = 2;  // 幂等键，本次扣费唯一
  string amount      = 3;  // 十进制字符串，> 0
  string description = 4;  // 必填，扣费原因
  string extra       = 5;  // 可选，jsonb 文本，你自存的数据
}
message DeductResponse {
  bool   applied       = 1;  // false = 幂等重放（之前已扣过）
  string balance_after = 2;  // 扣后余额
  string payer_account_id = 3;
  string balance_source = 4;
  int64 organization_id = 5;
  int64 authz_generation = 6;
}

message RefundRequest {
  string refund_request_id   = 1;  // 本次退款的幂等键，唯一
  string original_request_id = 2;  // 要冲销的原扣费 request_id
  string amount              = 3;  // 本次退多少（可小于原扣，支持多次部分退）
  string description         = 4;  // 必填，退款原因
  string extra               = 5;
}
message RefundResponse {
  bool   applied        = 1;
  string balance_after  = 2;
  string refunded_total = 3;  // 该原扣累计已退金额
  string payer_account_id = 4;
  string balance_source = 5;
  int64 organization_id = 6;
  int64 authz_generation = 7;
}

message GetBalanceRequest  { string account_id = 1; }
message GetBalanceResponse {
  string balance = 1;
  string payer_account_id = 2;
  string balance_source = 3;
  int64 organization_id = 4;
  int64 authz_generation = 5;
}

message Material {
  string id = 1; // 对外不透明素材 ID，不是数据库主键
  string account_id = 2;
  string file_name = 3;
  string url = 4;
  string content_type = 5;
  int64 size_bytes = 6;
  string kind = 7;
  string source = 8;
  string created_at = 9;
}
message ListMaterialsRequest {
  string account_id = 1;
  string kind = 2;
  string keyword = 3;
  int32 page = 4;
  int32 page_size = 5;
}
message ListMaterialsResponse {
  repeated Material items = 1;
  int64 total = 2;
  int32 page = 3;
  int32 page_size = 4;
}
message GetMaterialRequest { string account_id = 1; string id = 2; }
message UploadMaterialRequest {
  string account_id = 1;
  string file_name = 2;
  string content_type = 3;
  bytes data = 4;
}
message UploadMaterialResponse {
  Material material = 1;
  string file_url = 2;
}
message AddMaterialByUrlRequest {
  string account_id = 1;
  string url = 2;
}
message AddMaterialByUrlResponse { string material_id = 1; }
message RenameMaterialRequest {
  string account_id = 1;
  string id = 2;
  string file_name = 3;
}
message DeleteMaterialRequest { string account_id = 1; string id = 2; }
message DeleteMaterialResponse { string id = 1; bool deleted = 2; }
message BatchDeleteMaterialsRequest {
  string account_id = 1;
  repeated string ids = 2; // 最多 100 个
}
message BatchDeleteMaterialsResponse {
  repeated string deleted_ids = 1;
  int32 deleted_count = 2;
}
```

## 5. Go 客户端示例

```go
import (
    "context"
    "fmt"
    "trpc.group/trpc-go/trpc-go/client"
    pb "github.com/Wei-Shaw/sub2api/internal/rpc/innerpb" // 或你自己用 proto 生成的 stub
)

const innerAPIToken = "<你拿到的 token>"

func newProxy() pb.InnerAPIClientProxy {
    return pb.NewInnerAPIClientProxy(
        client.WithTarget("ip://10.0.0.5:9100"), // 平台 RPC 地址:端口
    )
}

// 扣费
func deduct(ctx context.Context) error {
    rsp, err := newProxy().Deduct(ctx, &pb.DeductRequest{
        AccountId:   "acct_root_abc123",
        RequestId:   "order-20260628-0001", // 你的业务单号，保证唯一
        Amount:      "12.34",
        Description: "订单 #0001 扣费",
        Extra:       `{"order_id":"0001","sku":"abc"}`,
    },
        client.WithMetaData("app-token", []byte(innerAPIToken)),
    )
    if err != nil {
        return err // 见第 7 节错误处理
    }
    _ = rsp.GetBalanceAfter()
    return nil
}

// 部分退
func refund(ctx context.Context) error {
    _, err := newProxy().Refund(ctx, &pb.RefundRequest{
        RefundRequestId:   "refund-20260628-0001-a", // 本次退款唯一键
        OriginalRequestId: "order-20260628-0001",     // 原扣单号
        Amount:            "4.00",
        Description:       "订单 #0001 部分退款",
    },
        client.WithMetaData("app-token", []byte(innerAPIToken)),
    )
    return err
}
```

> 非 Go 语言：需要支持 trpc 协议的客户端。多数接入方用 Go，其它语言请联系平台方。

素材写入、重命名和删除示例：

```go
// 上传：data 是文件原始字节，单文件大小和类型由平台素材服务校验。
uploaded, err := newProxy().UploadMaterial(ctx, &pb.UploadMaterialRequest{
    AccountId:   "acct_root_abc123",
    FileName:    "reference.png",
    ContentType: "image/png",
    Data:        fileBytes,
}, client.WithMetaData("app-token", []byte(innerAPIToken)))
if err != nil {
    return err
}
fmt.Println(uploaded.GetFileUrl(), uploaded.GetMaterial().GetId(), uploaded.GetMaterial().GetSizeBytes())

// 已有同一 COS 公网域名下的 URL 时，可以直接加入素材库。
// File API 返回的临时 URL 会被移动到该用户的正式素材目录。
added, err := newProxy().AddMaterialByUrl(ctx, &pb.AddMaterialByUrlRequest{
    AccountId: "acct_root_abc123",
    Url:       developerFileURL,
}, client.WithMetaData("app-token", []byte(innerAPIToken)))
if err != nil {
    return err
}

renamed, err := newProxy().RenameMaterial(ctx, &pb.RenameMaterialRequest{
    AccountId: "acct_root_abc123",
    Id:        added.GetMaterialId(),
    FileName:  "新的展示名称.png",
}, client.WithMetaData("app-token", []byte(innerAPIToken)))
if err != nil {
    return err
}

// 删除：按 account_id + 素材 id 删除，只能删除该用户自己的素材。
_, err = newProxy().DeleteMaterial(ctx, &pb.DeleteMaterialRequest{
    AccountId: "acct_root_abc123",
    Id:        renamed.GetId(),
}, client.WithMetaData("app-token", []byte(innerAPIToken)))
```

`RenameMaterial` 只修改展示名称，不修改对象键或 URL。`DeleteMaterial` 和 `BatchDeleteMaterials` 是软删除，素材记录会从用户素材列表中消失；对象存储清理由平台后台任务处理。上述写接口都需要 `materials:write`，且不允许跨用户操作。

File API 上传临时文件并通过 `AddMaterialByUrl` 转入素材库的完整流程，见 [开发者密钥与 File API 接入文档](./DEVELOPER_KEY_FILE_API_INTEGRATION.md)。

## 6. 幂等约定（重要）

| 场景 | 规则 |
|------|------|
| 扣费重试 | 用**同一个 `request_id`** 重发，保证至多扣一次；重放时 `applied=false` 且返回首次的余额 |
| 退款重试 | 用**同一个 `refund_request_id`** 重发，保证至多退一次 |
| 多次部分退 | 每次用**不同的 `refund_request_id`**，`original_request_id` 都指向同一原扣；累计退款不能超过原扣金额 |

- `request_id` / `refund_request_id` 建议用你自己的业务单号，长度 ≤ 128。
- **网络超时/结果不确定时，请用同一个幂等键重试**，不要换新键——换新键会被当成新的一笔。

## 7. 错误处理

错误通过 tRPC 错误码返回，用 `errs.Code(err)` / `errs.Msg(err)` 读取：

| 情况 | 表现 | 你该怎么做 |
|------|------|-----------|
| 余额不足（不透支） | `INSUFFICIENT_BALANCE` | 提示充值，不重试 |
| 金额非法 / 原因为空 | `LEDGER_INVALID_AMOUNT` / `LEDGER_DESCRIPTION_REQUIRED` | 修正参数 |
| 用户不存在 / 原扣不存在 | `USER_NOT_FOUND` / `ORIGINAL_DEDUCT_NOT_FOUND` | 检查 account_id / original_request_id |
| 累计退款超原扣 | `OVER_REFUND` | 核对已退金额 |
| 鉴权失败 / app 停用 / token 已被刷新作废 | `RetServerAuthFail` | 检查/更换 token，联系管理员 |
| 同 request_id 参数不一致 | `LEDGER_REQUEST_CONFLICT` | 同一幂等键的参数必须一致 |
| 服务端未配密钥 | `RetServerSystemErr` | 联系平台方 |

**重试策略**：只对网络错误 / 超时 / `RetServerSystemErr` 这类瞬时错误重试（用同一幂等键）；对余额不足、参数错误、超额退这类业务错误**不要重试**。

## 8. 边界与约束

- **不透支**：余额不足直接拒绝，绝不会把余额扣成负数。
- **只能退自己扣的**：`Refund` 只能冲销**你这个 app 通过本 RPC 扣的**流水。
- **金额精度**：服务端按 `decimal(20,8)` 存储，传字符串避免精度问题。
- **审计**：每笔扣/退都在平台侧落永久流水（含你的 `app_id`、`description`、`extra`），可用于对账；管理员可查看你这个 app 的累计扣费。
- **停用 / 刷新 / 删除即时生效**：管理员停用、刷新 token 或删除你的 app 后，最多几秒内所有旧调用开始被拒。

## 9. 快速自测清单

1. 拿到 `app_id` + `token`。
2. `GetBalance(user)` 能读到余额 → 鉴权通了。
3. `Deduct(user, req=A, "1.00", "test")` → 余额减 1。
4. 用**同样** `req=A` 再扣一次 → `applied=false`，余额不变（幂等 OK）。
5. `Refund(refund=R1, orig=A, "0.40", "test")` → 余额加 0.4，`refunded_total=0.40`。
6. `Refund(refund=R2, orig=A, "1.00", ...)` → `OVER_REFUND`（0.4+1.0 > 1.0）。
