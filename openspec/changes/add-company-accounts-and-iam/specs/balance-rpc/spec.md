## MODIFIED Requirements

### Requirement: 扣费（不透支、幂等、原因必填）

系统在 `Deduct` 时 SHALL 将请求的 `user_id` 视为消费用户，并通过统一计费上下文解析实际付款用户。个人/主账号与无共享余额权限的 IAM 用户 SHALL 以自身为付款用户；拥有有效 `CompanySharedBalanceUse` 权限的 IAM 用户 SHALL 以公司主账号为付款用户。`Deduct` SHALL 仅扣减实际付款用户的 `users.balance`，不触碰 apikey 配额/限流/账号配额/订阅用量。`Deduct` SHALL 要求 `amount > 0` 且 `description` 非空，否则返回参数错误。

系统 SHALL NOT 允许实际付款用户透支：当付款用户余额小于扣费金额时，SHALL 拒绝本次扣费，不得回退到另一余额来源，也不得把任何余额扣成负数。

系统 SHALL 以 `(app_id, request_id)` 为幂等键：同一 app 重复提交相同 `request_id` 时 SHALL 不重复扣费，并返回与首次一致的结果（含扣后余额与付款方快照）。

每次成功扣费 SHALL 在 `balance_ledger` 落一条 `kind=deduct` 永久流水，至少包含 `app_id`、消费 `user_id`、`organization_id`、`payer_user_id`、`balance_source`、权限版本、`amount`、`description`、`extra` 与付款方扣后余额快照。该流水 SHALL NOT 被 TTL 清理。

#### Scenario: 余额充足时扣费成功

- **WHEN** 合法 app 调用 `Deduct(user_id, request_id=R1, amount=A, description=非空)` 且解析出的付款用户余额 ≥ A
- **THEN** 系统 SHALL 把付款用户余额减少 A
- **AND** 落一条 `kind=deduct`、`request_id=R1` 且含消费方与付款方快照的流水
- **AND** 返回付款方扣后余额

#### Scenario: 共享余额 IAM 用户扣费

- **WHEN** `user_id` 属于拥有有效共享余额权限的 IAM 用户
- **THEN** 系统 SHALL 扣减该组织主账号余额
- **AND** SHALL NOT 扣减 IAM 用户的划拨余额

#### Scenario: 选定付款方余额不足且不回退

- **WHEN** 解析出的付款用户余额 < A
- **THEN** 系统 SHALL 返回 INSUFFICIENT_BALANCE（FAILED_PRECONDITION）
- **AND** 消费用户与付款用户余额均保持不变
- **AND** SHALL NOT 尝试另一余额来源
- **AND** 不落下“已扣”的脏流水

#### Scenario: 相同 request_id 幂等重放

- **WHEN** 同一 app 用相同 `request_id=R1` 再次调用 `Deduct`
- **THEN** 系统 SHALL NOT 再次扣减余额或重新解析付款方
- **AND** 返回与首次一致的付款方和扣后余额快照

#### Scenario: 缺少扣费原因被拒

- **WHEN** `description` 为空
- **THEN** 系统 SHALL 返回 INVALID_ARGUMENT 且不扣费

### Requirement: 退费（部分退、凭原流水冲销、双幂等）

系统在 `Refund` 时 SHALL 支持部分退：调用方提供本次退费 `amount`，系统 SHALL 凭 `original_request_id` 定位本 app 自己的原 `kind=deduct` 流水并对其反向冲销。退款 SHALL 始终增加原流水快照中的 `payer_user_id` 余额，不得依据退款时的组织关系、用户状态或权限重新选择付款方。系统 SHALL NOT 允许某原扣的累计已退金额超过其原扣金额。

系统 SHALL 仅允许冲销由本 `app_id` 经本 RPC 产生的扣费流水；对不存在的原扣流水 SHALL 返回未找到。`Refund` 同样 SHALL 要求 `amount > 0` 且 `description` 非空。

系统 SHALL 以调用方提供的 `refund_request_id` 作为本笔退费的幂等键：重复提交相同 `refund_request_id` SHALL 不重复退款。每次成功退费 SHALL 落一条 `kind=refund` 流水，包含 `refund_of=original_request_id`、原消费用户、组织、原付款用户和余额来源快照、`description`、`extra` 与付款方退后余额快照，并更新原 deduct 流水的累计已退金额。

#### Scenario: 部分退成功

- **WHEN** 原扣流水 `R1` 金额为 10、已退 0，合法 app 调用 `Refund(refund_request_id=F1, original_request_id=R1, amount=4, description=非空)`
- **THEN** 系统 SHALL 把 `R1` 快照的付款用户余额增加 4
- **AND** 原扣流水 `R1` 的累计已退金额 SHALL 变为 4
- **AND** 落一条 `kind=refund`、`refund_of=R1` 的流水

#### Scenario: 退款期间权限已变化

- **WHEN** 原扣使用主账号共享余额，但退款时 IAM 用户已失去共享余额权限
- **THEN** 系统 SHALL 仍把退款记入原扣快照的主账号
- **AND** SHALL NOT 把退款记入 IAM 用户划拨余额

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

系统 SHALL 在每次扣费/退费的数据库事务提交之后，同步更新实际付款用户的 Redis 余额缓存（写入扣/退后的新余额，或失效该缓存），使后续 `GetBalance` 与网关 preflight 立即读到最新余额。`GetBalance` SHALL 对所请求的消费用户解析当前实际付款方，并向已鉴权的计费 app 返回付款方当前余额、付款方 ID 与余额来源；面向终端用户的 HTTP/前端映射 SHALL 仅在当前身份具备 `CompanyFinanceReadOnly` 或组织 owner 权限时返回主账号余额金额，否则只能返回自身金额和非数值的余额来源标识。

#### Scenario: 扣费后付款方缓存立即反映新余额

- **WHEN** 一次 `Deduct` 成功提交
- **THEN** 系统 SHALL 在返回前同步更新或失效实际付款用户的余额缓存
- **AND** 紧随其后的计费 `GetBalance` SHALL 返回付款方扣后余额

#### Scenario: 退费后付款方缓存立即反映新余额

- **WHEN** 一次 `Refund` 成功提交
- **THEN** 系统 SHALL 同步更新或失效原流水付款用户的余额缓存
- **AND** 紧随其后的计费 `GetBalance` SHALL 返回付款方退后余额

#### Scenario: 共享余额不泄漏给终端 IAM 用户

- **WHEN** 仅拥有共享余额权限的 IAM 用户读取其账户信息
- **THEN** 面向用户的响应 SHALL NOT 包含 `GetBalance` 解析出的主账号余额金额
- **AND** MAY 返回 `balance_source=shared`
