# 受限管理员 API 密钥

受限管理员密钥用于自动化管理指定资源。它与调用模型使用的用户 API Key 不同，也不等同于管理员登录会话。通过请求头 `x-api-key` 使用；只有权限目录明确登记的接口和操作才会开放，新增接口不会自动继承权限。

## 创建与配置

管理员登录后，在「系统设置 → 安全」的管理员访问密钥区域创建密钥。权限目录和分组加载成功后才能创建。

1. 填写名称、备注、到期时间及允许的 IP/CIDR。到期时间使用本地时间，留空表示永不过期；IP 列表留空表示不限制来源 IP。
2. 添加资源规则，分别选择操作、分组、账户类型和平台。新建规则不勾选任何操作或账户类型，全站权限也全部关闭。
3. 按需独立勾选全站权限，保存后立即复制完整密钥。完整密钥仅在创建或轮换的当前响应中出现，关闭展示后不能从列表重新读取。

创建、更新、停用/启用、轮换、撤销和删除记录均沿用系统二次验证流程；是否弹出验证由系统的 step-up 配置和当前授权状态决定。密钥生命周期接口只接受管理员 JWT 会话，受限密钥不能管理自身或创建其他管理员密钥。

已有旧版全权密钥时，页面显示兼容折叠区，可轮换或删除。页面不再提供新建旧版全权密钥的入口；应将自动化任务迁移到受限密钥。

## 账户规则如何匹配

对于同一项账户操作，一条规则内部必须同时满足以下条件（AND）：

- 该操作出现在 `permissions` 中。
- 账户类型出现在 `account_types` 中。空列表表示没有账户操作权限，而非所有类型。
- `platforms` 为空，或者账户平台命中其中一项。
- 已分组账户至少命中一个所选分组，或规则启用了 `all_groups`；未分组账户必须显式启用 `include_ungrouped`。

多条规则之间是 OR：任一完整规则允许该项操作即可。不能将一条规则中的账户类型、另一条规则中的分组或平台拼起来授权。不同操作会分别检查其所需权限；写入凭证、修改网络、状态、调度、模型和分组关联不是「编辑名称与备注」的附带能力。

`all_groups` 包含未来新增的分组，但不自动包含未分组账户。新建账户的每个目标分组、以及调整关联时新增或移除的目标分组，还必须通过对应操作的分组检查。

**共享账户仍是一个账户资源。** 账户关联多个分组时，命中其中一个授权分组可能允许对该账户执行已授权操作。改凭证、改状态、刷新或删除会影响所有使用该账户的分组。界面会明确提示这一点；请按共享影响授予操作。受限账户响应只返回允许读取的分组关联，不代表该账户没有其他关联。

## 分组模型是实际准入限制

`groups.models.read` 和 `groups.models.write` 是资源规则中的独立权限。它们按同一规则的分组与平台检查，账户类型不限制整个分组的配置，也不要求同时授予账户操作或全站权限。

此处修改的是上游 `model_allowlist`，会影响整个分组可调用的模型，不能当作只改变前端展示的列表。写接口只允许单独提交 `model_allowlist`，不能借此修改费率等其他分组设置。只授予分组模型读取时，响应仅提供分组 ID、名称、平台和模型白名单；授予全站 `groups.read` 后才可读取完整分组配置。

读取指定分组的候选模型须有该分组的读取授权，或者全站 `groups.read`。读取默认模型候选（分组 ID 为 `0`）须独立授予 `models.defaults.read`。

## 全站权限和响应边界

`global_permissions` 与资源规则独立，不受所选分组或账户类型约束。例如 `users.read`、`dashboard.read`、`payment.read` 可读取全站相应数据。选择某个分组不会将这些权限限制为该分组；反过来，选择分组也不会隐式获得这些权限。

权限目录按接口与 HTTP 方法精确登记。当前不提供 `payment.mark_paid` 或退款预览权限；不要把退款、取消订单、重新履约等其他权限当作这些动作的替代。

受限响应按用途收窄：

- 账户读取返回基本管理信息和模型映射，不返回 OAuth token、API 密钥、任意 `extra` 或完整凭证。`accounts.credentials` 是写入权限，不提供读取凭证的能力。
- 代理候选包含 ID、名称、协议、主机、端口、状态与到期时间，不包含代理认证用户名或密码。
- 用户资料采用字段白名单；联系方式需要 `users.read`，余额字段需要 `users.balance.read`，分组配置字段需要 `groups.read`。用户响应不嵌入订阅和用量关联数据，这些能力由各自接口及权限控制。
- 订阅响应不嵌入用户、分配管理员或完整分组；仅有订阅或平台额度写权限时，成功响应只确认资源 ID，不附带独立读取权限的数据。
- 用量记录仅返回请求自身的用量、计费快照及关联 ID，不嵌入用户 API Key、账户凭证或用户财务资料。优惠码使用记录及公告阅读记录也只返回记录字段与用户 ID。
- 请求错误详情等敏感运维数据有单独权限。全站读取权限仍可能包含业务敏感数据，应只授予任务确实需要的项。

## 密钥存储、轮换与撤销

完整密钥由随机字节生成，数据库保存 SHA-256 摘要及用于展示和定位的前缀，不保存可供重显的完整密钥。客户端应把密钥放入密钥管理器或运行环境的秘密变量中。

停用可再次启用；到期或不符合允许 IP 的密钥不能认证。轮换立即使旧密钥失效并返回一次性新密钥。永久撤销不可恢复。只有已撤销密钥才可以删除列表记录；删除只移除管理列表展示，历史操作记录继续保留。

编辑时若没有修改到期时间，前端会保留服务端原始值，避免将秒数、微秒或时区信息截断。

## 代理查询与自动选择

`proxies.read` 是全站权限，与账户的 `accounts.network` 分开。前者用于获取安全候选信息，后者控制为授权账户设置接入地址、代理与连接选项。

- `GET /api/v1/admin/proxies` 使用 `page`、`page_size` 和 `search`，默认每页 50，最多 100。
- `GET /api/v1/admin/proxies/all` 为兼容客户端返回数组；显式传入分页参数时按分页查询，分页信息在 `X-Total-Count`、`X-Page`、`X-Page-Size` 和 `X-Total-Pages` 响应头中。
- `/proxies/all` 未传分页参数时，从全池重新随机抽样最多 50 个候选，不排除已被账户使用的代理；响应设置 `Cache-Control: no-store`。
- `ids` 可回显已选代理，一次最多 100 个不同 ID，按请求顺序返回实际存在的记录。

受限账户写入与 Codex 导入支持 `proxy_mode`：`direct`（直连）、`specified`（正数 `proxy_id`）、`random`（随机）、`least_used`（最少使用）。自动选择由服务端在账户写事务内从启用且未过期的代理中完成，无可用代理则失败。自动模式不能同时指定正数 `proxy_id`；旧字段 `random_proxy: true` 兼容随机模式，不能与其他模式冲突。随机候选查询与写入时自动选择是两个独立行为。

## Codex 会话导入

受限密钥可使用 `POST /api/v1/admin/accounts/import/codex-session`，但不会回退到全权管理员导入流程。

- 必须明确提供 `group_ids`；新账户固定为 `openai` 平台、`oauth` 类型，目标分组逐一检查。
- 新建需要 `accounts.create`、`accounts.credentials` 和 `accounts.schedule`。更新匹配账户会重新检查当前资源和对应写权限；调整关联还需要 `accounts.groups`。
- `base_url`、代理、指纹或 WebSocket 连接配置需要 `accounts.network`；`model_mapping` 需要 `accounts.models.write`。
- `credential_extras` 只接受独立授权的 `base_url`、`model_mapping`；`extra` 只开放已登记的 Codex 指纹及 WebSocket 选项，不支持任意字段透传或跳过混合渠道风险检查。
- 导入只按当前条目的身份查询受限候选，响应不返回候选凭证。更新已有账户时仍会在写入阶段重验身份与权限。

## API 示例

以下域名、密钥、JWT、ID 和会话内容均为占位符。通用成功响应使用系统现有响应封装。

管理员会话创建受限密钥（如系统要求 step-up，应先完成当前二次验证流程）：

```bash
curl 'https://example.invalid/api/v1/admin/settings/admin-api-keys' \
  -H 'Authorization: Bearer <ADMIN_JWT>' \
  -H 'Content-Type: application/json' \
  --data '{
    "name": "只读 OAuth 工具",
    "description": "读取指定分组的 OpenAI OAuth 账户",
    "enabled": true,
    "expires_at": null,
    "allowed_ips": ["192.0.2.10/32"],
    "policy": {
      "rules": [{
        "permissions": ["accounts.read"],
        "group_ids": [123],
        "all_groups": false,
        "include_ungrouped": false,
        "account_types": ["oauth"],
        "platforms": ["openai"]
      }],
      "global_permissions": []
    }
  }'
```

使用受限密钥读取账户：

```bash
curl 'https://example.invalid/api/v1/admin/accounts?page=1&page_size=20' \
  -H 'x-api-key: <SCOPED_ADMIN_KEY>'
```

已授予所选分组的 `groups.models.write` 后，单独更新其模型白名单：

```bash
curl -X PUT 'https://example.invalid/api/v1/admin/groups/123' \
  -H 'x-api-key: <SCOPED_ADMIN_KEY>' \
  -H 'Content-Type: application/json' \
  --data '{"model_allowlist":{"enabled":true,"models":["<MODEL_ID>"]}}'
```

具备上述导入所需权限后，新建 Codex OAuth 账户并由服务端随机选择代理：

```bash
curl 'https://example.invalid/api/v1/admin/accounts/import/codex-session' \
  -H 'x-api-key: <SCOPED_ADMIN_KEY>' \
  -H 'Content-Type: application/json' \
  --data '{
    "content": "<CODEX_SESSION_CONTENT>",
    "name": "导入账户",
    "group_ids": [123],
    "update_existing": false,
    "proxy_mode": "random"
  }'
```

生命周期接口位于 `/api/v1/admin/settings/admin-api-keys`：`GET` 列表及目录、`POST` 创建、`PUT /<KEY_ID>` 更新、`POST /<KEY_ID>/rotate` 轮换、`DELETE /<KEY_ID>` 永久撤销、`DELETE /<KEY_ID>/record` 删除已撤销记录。更新请求包含完整配置与策略；不要将未提交字段当作保持原值的 PATCH 语义。
