# Inner API RPC 设计方案

## 目标

将现有仅提供余额账本能力的 `balance_rpc` 重构为通用的内部服务 tRPC，统一服务间身份、网络入口和权限模型。当前没有线上接入方和历史数据，因此直接使用新命名和新表结构，不保留旧配置、旧类型或旧表兼容层。

## 对外命名

- 配置段：`inner_api_rpc`
- 服务生命周期类型：`InnerAPIRPCServer`
- 构造函数：`NewInnerAPIRPCServer`
- 应用字段：`Application.InnerAPIRPC`
- tRPC 服务名：`sub2api.inner.v1.InnerAPI`
- 管理对象：`InnerAPIApp`
- 管理路由：`/api/v1/admin/inner-api-apps`
- 数据表：`inner_api_apps`

旧的 `balance_rpc`、`BalanceRPCServer`、`BillingApp` 和 `billing_apps` 只保留在历史迁移文件或历史归档文档中，不作为运行时代码接口。

## 接入方权限

创建内部 app 时显式提交权限集合，只允许以下四项：

```text
balance:write
balance:read
materials:read
materials:write
```

权限存储在 `inner_api_apps.permissions` JSONB 字段中。创建、列表和详情接口返回权限集合；刷新 token、停用、删除行为保持原有语义。

### 方法权限映射

| RPC 方法 | 所需权限 |
|---|---|
| `Deduct`、`Refund` | `balance:write` |
| `GetBalance` | `balance:read` |
| 素材列表、素材详情、下载 URL | `materials:read` |
| 素材上传、导入、删除 | `materials:write` |

素材对象路径不直接使用连续的数据库用户 ID。服务端读取用户的 `account_id`，将
`user_id + account_id` 与服务端密钥做 HMAC-SHA256 派生后生成用户目录，例如
`users/u_<opaque>/materials/...`。无法读取用户身份或路径密钥时拒绝上传，不降级到明文路径；
数据库中的 `cos_key` 继续保存完整对象路径，历史素材无需通过目录重新计算。

鉴权过滤器先验证 `app-token`，再把 app 身份和权限集合写入 context。每个 RPC 方法入口使用统一的 `RequirePermission` 检查，认证失败和授权失败分别返回统一的 tRPC 错误，不泄露内部实现细节。

## Token 与网络安全

Token 继续使用独立密钥生成的 AES-256-GCM 密文，数据库只存 app 注册信息、token 版本和权限，不存 token。刷新 token 会递增版本并立即使旧 token 失效。

RPC 端口必须部署在内网或服务网格中；单机调用绑定 `127.0.0.1`，跨主机调用使用防火墙白名单，生产环境可叠加 mTLS。权限检查不能替代网络隔离。

## 数据库迁移

不修改已有迁移文件。新增迁移将把历史迁移创建的空 `billing_apps` 表改名为 `inner_api_apps`，补充权限 JSONB 字段和索引。由于当前没有历史数据，不保留旧表兼容视图。

## 素材能力

素材 RPC 复用现有 `UserMaterialService`，所有操作都必须明确 `account_id` 或租户归属，并由服务端校验该 app 的授权范围。读取操作只返回元数据和短期预签名下载 URL；写入操作复用现有文件大小、类型和图片尺寸校验，禁止客户端指定任意对象存储 key。RPC 不接受或返回数据库 `user_id`。

## 管理接口

```text
GET    /api/v1/admin/inner-api-apps
POST   /api/v1/admin/inner-api-apps
PATCH  /api/v1/admin/inner-api-apps/:app_id/enabled
PATCH  /api/v1/admin/inner-api-apps/:app_id/permissions
POST   /api/v1/admin/inner-api-apps/:app_id/refresh-token
DELETE /api/v1/admin/inner-api-apps/:app_id
```

创建请求示例：

```json
{
  "app_name": "image-worker",
  "permissions": ["materials:read", "materials:write"]
}
```

Token 仍只在创建或刷新响应中返回一次。所有管理变更写入审计日志。
