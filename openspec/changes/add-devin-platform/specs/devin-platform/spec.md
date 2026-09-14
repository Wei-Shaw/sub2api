## Purpose

让管理员以 `devin` 平台创建/维护 Devin（Cognition）账号：PKCE 授权码登录、`devin-session-token$…` 会话 token 直贴、额度快照探测与连通性测试，并接入既有账号池（分组绑定、代理、并发、调度开关）。

## ADDED Requirements

### Requirement: Devin PKCE 授权链接生成
系统 SHALL 提供 `POST /api/v1/admin/devin/oauth/auth-url`：生成 PKCE verifier/challenge 与 state，返回授权 URL、session_id、state；会话（verifier、state、proxy）MUST 存于进程内 store 并设置过期时间；支持 `proxy_id` 指定出站代理。

#### Scenario: 正常生成
- **WHEN** 管理员调用 auth-url（可选 proxy_id）
- **THEN** 返回 `auth_url`（含 client_id/redirect_uri/code_challenge/state）、`session_id`、`state`
- **THEN** 会话可在后续 exchange-code 中被 session_id 取回

### Requirement: 授权码交换
系统 SHALL 提供 `POST /api/v1/admin/devin/oauth/exchange-code`：粘贴值为 PKCE 授权码时 MUST 校验 session_id/state 并依次尝试 `ExchangePKCEAuthorizationCode` → `ExchangeDevinCLIPKCECode`；粘贴值本身已是 api key（`devin-session-token$…`/JWT/长 hex）时 MUST 跳过交换且允许省略 session_id/state。交换成功后 SHOULD 调 `GetUserStatus` 拉取 email/org/plan 供建号 extra；GetUserStatus 失败 MUST NOT 阻断。

#### Scenario: 粘贴 session token 直贴
- **WHEN** code 为 `devin-session-token$…` 且未提供 session_id/state
- **THEN** 系统 MUST 直接返回该 token 作为 access_token，不调用交换 RPC

#### Scenario: state 不匹配
- **WHEN** PKCE code 的 state 与会话不符
- **THEN** 返回 400，不执行上游交换

#### Scenario: 会话过期
- **WHEN** session_id 不存在或已过期且 code 不是 api key 形态
- **THEN** 返回 400 提示重新生成授权链接

### Requirement: Devin 账号凭据模型
`platform=devin` 的账号 MUST 为 `type=oauth`；`credentials.access_token` 存 Connect api key；`credentials.api_server_url`/`client_version` 可选（缺省回落内置默认值）；`extra` 可存 `email`/`org_id`/`plan_name`/`devin_quota` 快照。

#### Scenario: 凭据缺省回落
- **WHEN** 账号未设置 api_server_url/client_version
- **THEN** 转发 MUST 使用内置默认 base URL 与 client version

### Requirement: Devin 额度探测
系统 SHALL 在账号用量接口中为 devin 账号返回 `devin_quota` 快照（计划名、credits、日/周剩余百分比与重置时间、ACU 用量）；探测失败 MUST 降级为带错误码的 UsageInfo，且错误结果缓存 MUST 短于成功缓存。

#### Scenario: 正常拉取
- **WHEN** 对 devin 账号查询用量
- **THEN** 返回 daily/weekly_quota_remaining_percent、reset_at、credits 与 acu 字段，并把原始快照合并入 extra.devin_quota

#### Scenario: 探测失败
- **WHEN** GetUserStatus 返回 unauthenticated/限流/传输错误
- **THEN** UsageInfo 携带机器可读错误码，且不抛出 500

### Requirement: 连通性测试
管理员测试端点 MUST 对 devin 账号调用 `GetUserStatus` 验证 token 有效性；成功返回账号身份信息，失败返回上游错误。

#### Scenario: token 有效
- **WHEN** 对 devin 账号执行连接测试
- **THEN** 返回成功与 email/org/plan 摘要
