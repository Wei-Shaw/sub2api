## ADDED Requirements

### Requirement: 独立端口的 tRPC-Go 余额账本服务

系统 SHALL 在与现有 HTTP API（`server.port`）不同的端口上，于同一进程内启动一个 tRPC-Go 服务 `BalanceLedger`，提供 `Deduct`、`Refund`、`GetBalance` 三个方法。该服务的监听端口 SHALL 由独立配置项（如 `balance_rpc.port`）决定；当该服务未启用时，系统 SHALL 不监听第二端口且现有 HTTP 行为不变。

金额参数 SHALL 以十进制字符串在 RPC 报文中传输，服务端转为数据库 `numeric` 处理，不得使用二进制浮点（`double`）承载金额。

#### Scenario: 第二端口与 HTTP 端口分离启动

- **WHEN** `balance_rpc.enabled=true` 且 `balance_rpc.port` 与 `server.port` 不同
- **THEN** 系统 SHALL 在 `server.port` 上提供现有 gin HTTP API
- **AND** 系统 SHALL 同时在 `balance_rpc.port` 上提供 tRPC-Go `BalanceLedger` 服务
- **AND** 进程收到关闭信号时 SHALL 同时优雅关闭两个监听

#### Scenario: 未启用时不影响现状

- **WHEN** `balance_rpc.enabled=false`
- **THEN** 系统 SHALL 不监听第二端口
- **AND** 现有 HTTP API 与网关计费路径行为不变

### Requirement: 扣费 app 接入与鉴权

系统 SHALL 以独立的接入方身份（`billing_apps`）对 RPC 调用鉴权，且 SHALL NOT 复用全局管理员 API Key（`admin_api_key`）。每个接入方 SHALL 拥有唯一 `app_id`，并可被管理员启用/停用。

鉴权采用**无状态 token**：接入方的 token SHALL 为本地密钥对 `app_id` 的 AES-256-GCM 密文（`token = AES-256-GCM(balance_rpc.encryption_key, payload{app_id})`），密钥独立于其它系统密钥；系统 SHALL NOT 在数据库存储任何 token 密文或其 hash。token 在创建时由系统签发并仅返回一次。

RPC 调用方 SHALL 通过 tRPC metadata 提供 token；系统 SHALL：先用本地密钥解密 token（解密成功即证明该 token 由本方签发），再校验 token 携带的版本号与该 app 当前 `token_version` 一致、且解密出的 `app_id` 对应的 app 存在且已启用。app 的启停状态与版本号 SHALL 可由本地缓存提供（短 TTL + 启停/刷新时主动失效），使鉴权热路径不必每请求查询数据库。解密失败、版本过期（token 已被刷新）、app 不存在或已停用时 SHALL 返回统一的未认证错误，不区分原因。

#### Scenario: 合法 token 调用通过

- **WHEN** metadata 中的 token 能用本地密钥成功解密，其版本号等于该 app 当前 `token_version`，且对应 app 存在且 `enabled=true`
- **THEN** 系统 SHALL 执行所请求的方法
- **AND** 本次产生的流水行 SHALL 记录该 `app_id`

#### Scenario: 非法或篡改 token 被拒

- **WHEN** token 无法用本地密钥解密（伪造 / 篡改 / 非本方密钥签发）
- **THEN** 系统 SHALL 返回未认证错误且不执行任何余额变更

#### Scenario: 已刷新作废的 token 被拒

- **WHEN** token 能解密，但其版本号小于该 app 当前 `token_version`（即管理员已刷新 token）
- **THEN** 系统 SHALL 返回未认证错误

#### Scenario: 停用的 app 被拒

- **WHEN** token 能解密，但对应 app `enabled=false`（含管理员刚停用、本地缓存已失效后重新加载到的状态）
- **THEN** 系统 SHALL 返回未认证错误

#### Scenario: 不复用管理员 API Key

- **WHEN** 调用方仅提供 `admin_api_key` 而无有效 token
- **THEN** 系统 SHALL 拒绝该 RPC 调用

### Requirement: 扣费（不透支、幂等、原因必填）

系统在 `Deduct` 时 SHALL 仅扣减 `users.balance`，不触碰 apikey 配额/限流/账号配额/订阅用量。`Deduct` SHALL 要求 `amount > 0` 且 `description` 非空，否则返回参数错误。

系统 SHALL NOT 允许透支：当用户余额小于扣费金额时，SHALL 拒绝本次扣费（返回余额不足），不得把余额扣成负数。

系统 SHALL 以 `(app_id, request_id)` 为幂等键：同一 app 重复提交相同 `request_id` 时 SHALL 不重复扣费，并返回与首次一致的结果（含扣后余额快照）。

每次成功扣费 SHALL 在 `balance_ledger` 落一条 `kind=deduct` 流水（含 `app_id`、`user_id`、`amount`、`description`、`extra`、扣后余额快照），该流水为永久审计记录，SHALL NOT 被 TTL 清理。

#### Scenario: 余额充足时扣费成功

- **WHEN** 合法 app 调用 `Deduct(user_id, request_id=R1, amount=A, description=非空)` 且用户余额 ≥ A
- **THEN** 系统 SHALL 把用户余额减少 A
- **AND** 落一条 `kind=deduct`、`request_id=R1` 的流水
- **AND** 返回扣后余额

#### Scenario: 余额不足拒绝且不透支

- **WHEN** 用户余额 < A
- **THEN** 系统 SHALL 返回 INSUFFICIENT_BALANCE（FAILED_PRECONDITION）
- **AND** 用户余额保持不变（不出现负数）
- **AND** 不落下"已扣"的脏流水

#### Scenario: 相同 request_id 幂等重放

- **WHEN** 同一 app 用相同 `request_id=R1` 再次调用 `Deduct`
- **THEN** 系统 SHALL NOT 再次扣减余额
- **AND** 返回与首次一致的扣后余额

#### Scenario: 缺少扣费原因被拒

- **WHEN** `description` 为空
- **THEN** 系统 SHALL 返回 INVALID_ARGUMENT 且不扣费

### Requirement: 退费（部分退、凭原流水冲销、双幂等）

系统在 `Refund` 时 SHALL 支持部分退：调用方提供本次退费 `amount`，系统 SHALL 凭 `original_request_id` 定位本 app 自己的原 `kind=deduct` 流水并对其反向冲销。系统 SHALL NOT 允许某原扣的累计已退金额超过其原扣金额。

系统 SHALL 仅允许冲销由本 `app_id` 经本 RPC 产生的扣费流水；对不存在的原扣流水 SHALL 返回未找到。`Refund` 同样 SHALL 要求 `amount > 0` 且 `description` 非空。

系统 SHALL 以调用方提供的 `refund_request_id` 作为本笔退费的幂等键：重复提交相同 `refund_request_id` SHALL 不重复退款。每次成功退费 SHALL 落一条 `kind=refund` 流水（含 `refund_of=original_request_id`、`description`、`extra`、退后余额快照），并更新原 deduct 流水的累计已退金额。

#### Scenario: 部分退成功

- **WHEN** 原扣流水 `R1` 金额为 10、已退 0，合法 app 调用 `Refund(refund_request_id=F1, original_request_id=R1, amount=4, description=非空)`
- **THEN** 系统 SHALL 把用户余额增加 4
- **AND** 原扣流水 `R1` 的累计已退金额 SHALL 变为 4
- **AND** 落一条 `kind=refund`、`refund_of=R1` 的流水

#### Scenario: 多次部分退累计不超原扣

- **WHEN** `R1` 金额 10、已退 4，再调用 `Refund(F2, R1, amount=6)`
- **THEN** 系统 SHALL 成功，累计已退变为 10

#### Scenario: 累计退款超额被拒

- **WHEN** `R1` 金额 10、已退 4，调用 `Refund(F3, R1, amount=7)`（4+7 > 10）
- **THEN** 系统 SHALL 返回 OVER_REFUND（FAILED_PRECONDITION）且余额不变

#### Scenario: 相同 refund_request_id 幂等重放

- **WHEN** 同一 app 用相同 `refund_request_id=F1` 再次调用 `Refund`
- **THEN** 系统 SHALL NOT 再次退款
- **AND** 返回与首次一致的结果

#### Scenario: 冲销不存在或非本 app 的扣费

- **WHEN** `original_request_id` 在本 app 名下没有对应的 `kind=deduct` 流水
- **THEN** 系统 SHALL 返回 NOT_FOUND 且余额不变

#### Scenario: 并发部分退不超额

- **WHEN** 两笔退款并发针对同一原扣 `R1`（金额 10、已退 0），各退 6
- **THEN** 系统 SHALL 串行化处理，使最终累计已退不超过 10（其中一笔成功、另一笔因超额被拒，或两笔合计 ≤ 10）

### Requirement: 缓存一致性

系统 SHALL 在每次扣费/退费的数据库事务提交之后，同步更新该用户的 Redis 余额缓存（写入扣/退后的新余额，或失效该缓存），使后续 `GetBalance` 与网关 preflight 立即读到最新余额。`GetBalance` SHALL 返回用户当前余额（缓存优先、未命中回源数据库）。

#### Scenario: 扣费后缓存立即反映新余额

- **WHEN** 一次 `Deduct` 成功提交
- **THEN** 系统 SHALL 在返回前同步更新或失效该用户的余额缓存
- **AND** 紧随其后的 `GetBalance` SHALL 返回扣后余额

#### Scenario: 退费后缓存立即反映新余额

- **WHEN** 一次 `Refund` 成功提交
- **THEN** 系统 SHALL 同步更新或失效该用户的余额缓存
- **AND** 紧随其后的 `GetBalance` SHALL 返回退后余额

### Requirement: 接入方管理

系统 SHALL 提供管理员能力来创建、启用/停用、刷新 token、删除扣费 app，以及查看其累计扣费：

- 创建时 SHALL 返回一次性 token（本地密钥对 `app_id` + 版本号的 AES-256-GCM 密文）且数据库 SHALL NOT 存储该 token 或其 hash。
- 停用的 app SHALL 立即无法通过 RPC 鉴权。
- 刷新 token SHALL 自增该 app 的 `token_version` 并返回携带新版本的一次性 token；**旧 token SHALL 立即失效**。
- 删除 app 后其 token SHALL 立即失效；该 app 的历史流水 SHALL 保留（不随之删除）。
- 系统 SHALL 能返回某 app 的累计扣费统计（累计扣费、累计退费、净扣费、扣/退笔数），数据来源于该 app 在 `balance_ledger` 的流水。

#### Scenario: 创建 app 返回一次性 token

- **WHEN** 管理员创建一个扣费 app
- **THEN** 系统 SHALL 生成唯一 `app_id` 与一次性 token，token 仅在本次响应返回
- **AND** 数据库 SHALL NOT 存储 token 或其 hash（仅存 `app_id` / 名称 / 启停状态 / token 版本）

#### Scenario: 刷新 token 使旧 token 立即失效

- **WHEN** 管理员对某 app 刷新 token
- **THEN** 系统 SHALL 返回携带新版本的一次性新 token
- **AND** 该 app 的旧 token 后续 RPC 调用 SHALL 被拒绝（未认证）

#### Scenario: 删除 app 后 token 失效且流水保留

- **WHEN** 管理员删除某 app
- **THEN** 该 app 的 token 后续 RPC 调用 SHALL 被拒绝
- **AND** 该 app 已产生的 `balance_ledger` 流水 SHALL 仍然保留

#### Scenario: 查看 app 累计扣费

- **WHEN** 管理员查询某 app 的费用统计
- **THEN** 系统 SHALL 返回该 app 的累计扣费、累计退费、净扣费与扣/退笔数

#### Scenario: 停用 app 立即失效

- **WHEN** 管理员将某 app 置为 `enabled=false`
- **THEN** 该 app 后续 RPC 调用 SHALL 被拒绝（未认证）
