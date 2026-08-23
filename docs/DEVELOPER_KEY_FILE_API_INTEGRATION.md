# 开发者密钥与 File API 接入文档

本文面向需要通过 HTTP 上传、删除临时文件的接入方。File API 使用用户级开发者密钥鉴权；上传成功后，可将返回的 URL 传给 Inner API 的 `AddMaterialByUrl`，把临时文件移动到该用户的素材库。

## 1. 接入流程

```text
创建开发者密钥
    -> POST /api/v1/file/ 上传临时文件
    -> 获得公开访问 URL
    -> 业务使用该 URL，或通过 AddMaterialByUrl 加入素材库
    -> 不再需要的临时文件调用 DELETE /api/v1/file/ 删除
```

需要注意：

- 开发者密钥只允许访问 `/api/v1/file/`，不能访问模型、账户、素材库等其他 API。
- 开发者密钥属于创建它的用户，上传和删除操作都限制在该用户的临时文件目录内。
- File API 上传的是临时文件，不会自动创建素材库记录。
- `AddMaterialByUrl` 是 Inner API RPC 方法，使用 Inner API app token 和 `materials:write` 权限，不使用开发者密钥。
- 服务端必须已启用并完整配置 COS，且对象存储实现支持流式上传和对象复制。

本文示例使用以下环境变量：

```bash
export BASE_URL="https://api.example.com"
export DEVELOPER_KEY="dev_替换为实际密钥"
```

## 2. 创建和管理开发者密钥

### 2.1 在管理后台创建

登录管理后台后，打开右上角个人信息菜单，点击「开发者密钥」。在弹窗中可以创建、查看和删除当前用户的开发者密钥。

密钥明文只在创建成功时展示一次。关闭弹窗后无法再次查看，丢失时需要删除旧密钥并重新创建。

### 2.2 通过 API 管理

密钥管理接口使用正常登录后获得的 JWT，而不是开发者密钥：

```http
Authorization: Bearer <USER_JWT>
```

| 方法 | 路径 | 作用 |
|------|------|------|
| `GET` | `/api/v1/user/developer-keys` | 查询当前用户的密钥 |
| `POST` | `/api/v1/user/developer-keys` | 创建密钥 |
| `DELETE` | `/api/v1/user/developer-keys/:id` | 删除并立即吊销密钥 |

创建密钥：

```bash
curl -X POST "${BASE_URL}/api/v1/user/developer-keys" \
  -H "Authorization: Bearer ${USER_JWT}" \
  -H "Content-Type: application/json" \
  -d '{"name":"自动化上传工具"}'
```

`name` 会去除首尾空白，不能为空，最长为 100 个 Unicode 字符。创建成功返回 HTTP `201`：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "key": {
      "id": 1,
      "name": "自动化上传工具",
      "key_prefix": "dev_xxxxxxxx",
      "created_at": "2026-08-23T08:00:00Z",
      "updated_at": "2026-08-23T08:00:00Z"
    },
    "secret": "dev_完整密钥仅在这里返回一次",
    "display_once": true
  }
}
```

其中 `secret` 是实际调用 File API 的凭证。`key_prefix` 只用于在后台辨认密钥，不能用于鉴权。服务端只保存完整密钥的 SHA-256 摘要，不保存可恢复的明文。

查询密钥：

```bash
curl "${BASE_URL}/api/v1/user/developer-keys" \
  -H "Authorization: Bearer ${USER_JWT}"
```

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "id": 1,
        "name": "自动化上传工具",
        "key_prefix": "dev_xxxxxxxx",
        "last_used_at": "2026-08-23T08:30:00Z",
        "created_at": "2026-08-23T08:00:00Z",
        "updated_at": "2026-08-23T08:00:00Z"
      }
    ]
  }
}
```

新密钥尚未使用时，`last_used_at` 字段可能不存在。列表不会返回密钥明文。

删除密钥：

```bash
curl -X DELETE "${BASE_URL}/api/v1/user/developer-keys/1" \
  -H "Authorization: Bearer ${USER_JWT}"
```

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "deleted": 1
  }
}
```

删除后，该密钥立即失效。

## 3. File API 鉴权

所有 File API 请求都必须携带完整开发者密钥：

```http
Authorization: Bearer dev_xxx
```

未提供凭证时返回 HTTP `401`：

```json
{
  "code": "DEVELOPER_KEY_REQUIRED",
  "message": "Developer key is required in the Authorization header"
}
```

格式错误、密钥不存在、密钥已删除或所属用户已停用时返回 HTTP `401`：

```json
{
  "code": "INVALID_DEVELOPER_KEY",
  "message": "Invalid developer key"
}
```

不要用用户 JWT 调用 File API，也不要用开发者密钥调用密钥管理接口或其他业务接口。

## 4. 上传文件

### 4.1 请求

```http
POST /api/v1/file/
Content-Type: multipart/form-data
Authorization: Bearer <DEVELOPER_KEY>
```

路径末尾的 `/` 是接口定义的一部分。表单中上传一个名为 `file` 的文件字段：

```bash
curl -X POST "${BASE_URL}/api/v1/file/" \
  -H "Authorization: Bearer ${DEVELOPER_KEY}" \
  -F "file=@./example.png"
```

### 4.2 成功响应

上传成功返回 HTTP `201`：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "url": "https://cdn.example.com/storage/assets/file_uploads/u_xxx/2026/08/550e8400-e29b-41d4-a716-446655440000.png",
    "size": 12345,
    "content_type": "image/png"
  }
}
```

字段说明：

| 字段 | 类型 | 说明 |
|------|------|------|
| `url` | string | 文件的 COS 公开访问 URL，也是删除或加入素材库时需要保存的值 |
| `size` | integer | 文件字节数 |
| `content_type` | string | 服务端最终使用的 MIME 类型 |

### 4.3 上传约束

- 单文件上限为 `512 MiB`，空文件会被拒绝。
- MIME 类型优先取 multipart 文件头；没有时按扩展名推断，仍无法识别时使用 `application/octet-stream`。
- 客户端文件名只用于提取安全扩展名，服务端会生成 UUID 对象名。
- 路径分隔符、绝对路径和目录穿越内容不会影响实际对象目录。
- 不安全或过长的扩展名会替换为 `.bin`。
- 每次上传都会生成一个新 URL，上传操作不具备幂等性。

文件在 COS 中的对象键结构为：

```text
<COS prefix>/file_uploads/<用户隔离前缀>/<yyyy>/<mm>/<uuid>.<ext>
```

`<用户隔离前缀>` 由服务端生成，不应由调用方构造或枚举。

## 5. 删除临时文件

### 5.1 请求

```http
DELETE /api/v1/file/
Content-Type: application/json
Authorization: Bearer <DEVELOPER_KEY>
```

请求体传入上传接口原样返回的 URL：

```bash
curl -X DELETE "${BASE_URL}/api/v1/file/" \
  -H "Authorization: Bearer ${DEVELOPER_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://cdn.example.com/storage/assets/file_uploads/u_xxx/2026/08/550e8400-e29b-41d4-a716-446655440000.png"}'
```

### 5.2 成功响应

删除成功返回 HTTP `200`：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "deleted": true
  }
}
```

### 5.3 删除范围

服务端会同时校验 URL 和对象归属：

- URL 的协议、域名、端口和基础路径必须与当前 COS 公网地址配置一致。
- URL 不能包含查询参数、fragment 或内嵌用户名密码。
- 只能删除当前开发者密钥所属用户的 `file_uploads` 临时文件。
- 不能删除其他用户的临时文件、正式素材文件或 COS 中的其他对象。
- 临时文件已经通过 `AddMaterialByUrl` 移入素材库后，原 URL 不再代表临时对象，不能再用 File API 删除。

## 6. 将临时文件加入素材库

File API 与 Inner API 可以组合使用：先以目标用户的开发者密钥上传，再通过有 `materials:write` 权限的 Inner API app 调用 `AddMaterialByUrl`。

请求中的 `account_id` 必须对应开发者密钥所属的同一用户：

```proto
message AddMaterialByUrlRequest {
  string account_id = 1;
  string url = 2;
}

message AddMaterialByUrlResponse {
  string material_id = 1;
}
```

Go 调用示例：

```go
result, err := innerAPI.AddMaterialByUrl(ctx, &innerpb.AddMaterialByUrlRequest{
    AccountId: "acct_root_abc123",
    Url:       uploadedURL, // POST /api/v1/file/ 返回的 data.url
}, client.WithMetaData("app-token", []byte(innerAPIToken)))
if err != nil {
    return err
}
fmt.Println(result.GetMaterialId())
```

服务端识别到 URL 位于 `file_uploads` 后，会执行以下操作：

1. 校验临时文件属于 `account_id` 对应的用户，跨用户 URL 返回 `DEVELOPER_FILE_FORBIDDEN`。
2. 使用 COS `MoveFile` 将对象移动到用户正式素材目录。
3. 创建素材库记录，并保存移动后的 URL 和对象键。
4. 如果移动后数据库写入失败，服务端会尝试把对象移回原临时目录。

正式素材对象位于类似以下目录：

```text
users/<用户隔离前缀>/materials/<yyyy>/<mm>/...
```

移动成功后，原临时 URL 不应继续保存或使用，应以素材接口返回的数据为准。同一 COS 公网域名下但不属于 `file_uploads` 的普通 URL 会保留原有的“仅登记、不移动”行为。

Inner API 的 app token、权限和客户端初始化方式见 [内部 API RPC 接入文档](./INNER_API_RPC_INTEGRATION.md)。

## 7. 响应和错误处理

业务接口的标准成功结构为：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

进入业务处理后的标准错误结构为：

```json
{
  "code": 400,
  "message": "file exceeds the 512 MiB limit",
  "reason": "FILE_TOO_LARGE"
}
```

鉴权和限流由中间件直接返回，使用字符串 `code`，例如：

```json
{
  "code": "RATE_LIMITED",
  "message": "Too many requests, please slow down and try again later"
}
```

建议客户端先按 HTTP 状态码分类，再读取标准业务错误的 `reason` 或中间件错误的字符串 `code`。不要假设两种错误结构完全相同。

| HTTP 状态 | `reason` / `code` | 说明 |
|-----------|-------------------|------|
| `400` | `COS_NOT_CONFIGURED` | COS 未启用或配置不完整 |
| `400` | `FILE_TOO_LARGE` | 文件超过 512 MiB；部分 multipart 解析失败场景可能只返回普通 400 消息 |
| `400` | `EMPTY_FILE` 或无 `reason` | 文件为空或缺少 `file` 字段 |
| `400` | `INVALID_FILE_URL` | 删除 URL 不符合 COS 地址约束 |
| `401` | `DEVELOPER_KEY_REQUIRED` | 未提供开发者密钥 |
| `401` | `INVALID_DEVELOPER_KEY` | 密钥格式错误、已吊销或不可用 |
| `403` | `DEVELOPER_FILE_FORBIDDEN` | 文件不属于当前用户 |
| `429` | `RATE_LIMITED` | 超过平台配置的 File API 每分钟限额；按 `Retry-After` 响应头等待 |
| `500` | `COS_STREAMING_UNSUPPORTED` | 当前对象存储实现不支持流式上传，需要平台方处理 |

对参数和权限类 `4xx` 错误不要自动重试。网络错误和 `5xx` 可以使用退避策略有限重试，但上传重试会产生新的对象和 URL，调用方需要自行处理可能的重复文件。

## 8. 完整 Shell 示例

```bash
set -euo pipefail

: "${BASE_URL:?BASE_URL is required}"
: "${DEVELOPER_KEY:?DEVELOPER_KEY is required}"

UPLOAD_RESPONSE="$(curl --fail-with-body --silent --show-error \
  -X POST "${BASE_URL}/api/v1/file/" \
  -H "Authorization: Bearer ${DEVELOPER_KEY}" \
  -F "file=@./example.png")"

FILE_URL="$(printf '%s' "${UPLOAD_RESPONSE}" | jq -er '.data.url')"
printf 'uploaded: %s\n' "${FILE_URL}"

# 文件不再需要、且尚未加入素材库时执行：
curl --fail-with-body --silent --show-error \
  -X DELETE "${BASE_URL}/api/v1/file/" \
  -H "Authorization: Bearer ${DEVELOPER_KEY}" \
  -H "Content-Type: application/json" \
  --data "$(jq -cn --arg url "${FILE_URL}" '{url: $url}')"
```

生产接入时建议为 `curl` 或 HTTP 客户端设置连接和读取超时。

## 9. 安全与接入检查

- 将开发者密钥保存在密钥管理服务或环境变量中，不要提交到代码仓库。
- 不要把密钥写入 URL、应用日志、错误追踪上下文或前端代码。
- 泄露后立即在「开发者密钥」弹窗或删除接口中吊销。
- 记录上传返回的 `url`，后续删除和素材入库都应使用原始返回值，不要自行拼接 COS 路径。
- 确认上传用户与 `AddMaterialByUrl.account_id` 是同一用户。
- 将 HTTP `429` 的 `Retry-After` 纳入退避逻辑。
- 只重试网络错误和可恢复的 `5xx`；不要无条件重试上传请求。
- 将文件加入素材库后，更新业务侧保存的 URL，不再使用临时 URL。
