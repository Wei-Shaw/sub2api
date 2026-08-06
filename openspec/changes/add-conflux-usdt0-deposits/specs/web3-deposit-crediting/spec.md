## ADDED Requirements

### Requirement: 充值金额必须按固定 USD 规则精确计算
系统 SHALL 使用 USDT0 固定 6 位小数把链上 raw uint256 转换为十进制 Token 金额，并 MUST 按 `1 USDT0 = 1 USD balance`、手续费 0 计算 credited amount。整个链路 MUST NOT 把金额转换为 `float64`。

#### Scenario: 精确转换六位小数
- **WHEN** raw amount 为 `123456789`
- **THEN** token amount MUST 精确等于 `123.456789`
- **THEN** credited amount MUST 精确等于 `123.456789 USD`

#### Scenario: 最小链上单位
- **WHEN** raw amount 为 `1`
- **THEN** token amount MUST 精确等于 `0.000001`
- **THEN** 系统 MUST 按低于最低充值规则处理而不是舍入为 0 或 0.01

#### Scenario: 金额超出平台表示范围
- **WHEN** Token 金额不能安全表示为平台 `DECIMAL(20,8)` 余额
- **THEN** 系统 MUST NOT 自动入账
- **THEN** 充值 MUST 进入人工审核或失败安全状态并触发告警

### Requirement: 系统必须按固定阈值决定充值处置
系统 SHALL 使用最低充值 `1 USDT0` 和自动入账上限 `10,000 USDT0` 决定业务状态。正好 1 和正好 10,000 MUST 属于自动入账范围，只有大于 10,000 才进入人工审核。

#### Scenario: 低于最低金额
- **WHEN** finalized 充值金额小于 `1.000000 USDT0`
- **THEN** 系统 MUST 标记为 `below_minimum`
- **THEN** MVP MUST NOT 累计该金额或自动增加余额

#### Scenario: 自动入账金额
- **WHEN** finalized 充值金额大于等于 1 且小于等于 10,000 USDT0，并且用户状态允许自动入账
- **THEN** 系统 MUST 将充值转换为 `ready_to_credit`

#### Scenario: 超过自动入账上限
- **WHEN** finalized 充值金额大于 `10,000.000000 USDT0`
- **THEN** 系统 MUST 将充值转换为 `manual_review`
- **THEN** 系统 MUST NOT 在管理员批准前增加余额

### Requirement: 余额入账必须具有数据库事务原子性
系统 SHALL 在一个 PostgreSQL 事务内锁定充值和用户、创建余额流水、增加 `users.balance`、增加 `users.total_recharged` 并把充值标记为 `credited`。任一事务内步骤失败 MUST 回滚全部业务变更。

#### Scenario: 入账事务成功
- **WHEN** `ready_to_credit` 充值被成功履约
- **THEN** 系统 MUST 创建一条 `web3_deposit` 类型余额流水
- **THEN** 用户 balance 和 total_recharged MUST 增加相同 credited amount
- **THEN** 充值 MUST 记录 credited amount 和 credited time 并进入 `credited`

#### Scenario: 写入余额后提交失败
- **WHEN** 事务在更新用户余额后、提交前失败
- **THEN** 用户余额、total_recharged、ledger 和充值状态 MUST 全部保持事务前状态
- **THEN** 后续重试 MUST 能重新完成完整入账

### Requirement: 链上事件必须最多入账一次
系统 SHALL 为每个充值构造包含 `chain_id + lowercase tx_hash + log_index` 的稳定 idempotency key，并 MUST 使用充值行锁、状态条件和 ledger 唯一约束防止重复入账。

#### Scenario: 多个 Worker 并发履约
- **WHEN** 多个实例或 Worker 同时尝试履约同一充值
- **THEN** 最多一个调用 MUST 创建 ledger 并增加余额
- **THEN** 其他调用 MUST 幂等返回已完成或冲突状态

#### Scenario: 已入账充值被重复调用
- **WHEN** `credited` 充值再次进入重试或管理员重复操作
- **THEN** 系统 MUST 读取已有 ledger 并幂等返回
- **THEN** 用户余额 MUST NOT 再次增加

### Requirement: 入账失败必须可安全重试
系统 SHALL 使用持久状态、retry count、next retry time 和有界退避处理暂时性入账错误。旧 Worker MUST NOT 覆盖新 Worker 或管理员已经完成的状态转换。

#### Scenario: 暂时性数据库错误
- **WHEN** 入账因可重试数据库错误失败且事务未提交
- **THEN** 充值 MUST 进入可重试 `failed` 状态或释放为 `ready_to_credit`
- **THEN** 系统 MUST 设置有界 next retry time

#### Scenario: Worker 在 claim 后崩溃
- **WHEN** Worker 将充值置为 `crediting` 后崩溃且未提交入账事务
- **THEN** 租约过期后另一个 Worker MUST 能重新 claim
- **THEN** 重试 MUST 仍受 ledger 唯一键保护

### Requirement: Web3 充值不得伪装为普通支付订单
系统 MUST NOT 为 Web3 充值创建 `PaymentOrder` 或 `RedeemCode`，MVP MUST NOT 触发现有基于 PaymentOrder 的邀请返佣。现有普通支付订单、订阅、退款和充值码行为 MUST 保持不变。

#### Scenario: Web3 充值到账
- **WHEN** Web3 充值成功进入 credited
- **THEN** 数据库 MUST 存在 Web3 deposit 和 balance ledger 事实
- **THEN** 系统 MUST NOT 创建对应 PaymentOrder、RedeemCode 或 affiliate rebate 记录

#### Scenario: 普通支付充值到账
- **WHEN** 现有支付 provider 回调成功
- **THEN** 系统 MUST 继续使用原 PaymentOrder 和充值履约流程
- **THEN** Web3 Deposit 模块 MUST NOT 改变其状态或退款语义

### Requirement: 缓存和通知必须在事务提交后处理
系统 SHALL 在余额事务成功提交后失效用户余额缓存并发送充值成功通知。缓存或通知失败 MUST NOT 回滚余额，也 MUST NOT 导致重复入账。

#### Scenario: 缓存失效成功
- **WHEN** Web3 余额事务提交成功
- **THEN** 系统 MUST 请求失效对应用户的余额缓存
- **THEN** 后续读取 MUST 最终观察到新余额

#### Scenario: 通知发送失败
- **WHEN** 余额已经提交但充值成功通知发送失败
- **THEN** 充值 MUST 保持 credited
- **THEN** 系统 MAY 重试通知但 MUST NOT 再次增加余额

### Requirement: 人工审核必须重新验证链上事实
管理员批准 `manual_review` 充值前，系统 MUST 重新执行 finalized canonical verification。管理员 MUST NOT 修改充值金额、用户、Token、tx hash、log index 或 block hash。

#### Scenario: 管理员批准合法大额充值
- **WHEN** 管理员批准一条仍通过 finalized 验证的 manual_review 充值
- **THEN** 系统 MUST 将其转换为可入账状态并执行相同幂等余额事务
- **THEN** 系统 MUST 记录管理员、原因、旧状态和新状态

#### Scenario: 批准时链上验证失败
- **WHEN** 管理员批准时 canonical block、receipt 或目标日志不再匹配
- **THEN** 系统 MUST 拒绝批准
- **THEN** 系统 MUST NOT 创建 ledger 或增加余额
