## ADDED Requirements

### Requirement: 扫描器必须固定识别 Conflux eSpace 官方 USDT0
系统 SHALL 只接受 Conflux eSpace Mainnet `chain_id=1030` 上 Token 合约 `0xaf37E8B6C9ED7f6318979f56Fc287d76c30847ff` 产生的 ERC20 `Transfer(address,address,uint256)` 日志，并 MUST 使用固定 decimals `6`。

#### Scenario: 指定 Token 向用户地址转账
- **WHEN** 固定 USDT0 Token 合约产生转入已分配地址的合法 Transfer 日志
- **THEN** 系统 MUST 创建或读取该链上事件对应的充值记录
- **THEN** 充值事件 ID MUST 使用 `chain_id + tx_hash + log_index`

#### Scenario: 同名或错误 Token 向用户地址转账
- **WHEN** 其他 Token 合约即使返回 `USDT0`、`USDT` 或相似 symbol 并向用户地址转账
- **THEN** 系统 MUST NOT 将该日志识别为充值

#### Scenario: OFT 合约产生日志
- **WHEN** USDT0 OFT 合约而非指定 Token 合约产生日志
- **THEN** 系统 MUST NOT 将其识别为用户充值资产

### Requirement: RPC 节点必须通过链身份和能力预检
当 Web3 Deposit 功能启用时，系统 MUST 在运行前验证每个 RPC 节点的 chain ID、Token bytecode、Token decimals 和 `finalized` block tag 支持。错误链或能力不足的节点 MUST 被隔离。

#### Scenario: RPC 返回正确链参数
- **WHEN** RPC 返回 chain ID `0x406`、Token 地址存在 bytecode、`decimals()` 返回 6 且支持 finalized block
- **THEN** 节点 MAY 进入健康 endpoint 池

#### Scenario: RPC 连接到错误网络
- **WHEN** RPC 的 `eth_chainId` 不等于 `0x406`
- **THEN** 系统 MUST 将该节点标记为 unhealthy
- **THEN** 系统 MUST NOT 使用该节点发现、确认或批准充值

#### Scenario: 所有 RPC 节点不可用
- **WHEN** 所有配置 RPC 都未通过预检或运行中变为 unhealthy
- **THEN** 扫描器 MUST 停止推进游标
- **THEN** runtime MUST 暴露 unhealthy/degraded 状态并触发告警

### Requirement: Transfer 日志解析必须严格且精确
系统 SHALL 严格验证 Transfer topic 数量、topic0、地址编码、data 长度和 uint256 金额。raw amount MUST 使用任意精度整数解析，解析器 MUST NOT 使用 `float64` 或因畸形日志 panic。

#### Scenario: 解析标准 Transfer
- **WHEN** 日志具有标准 Transfer topic、两个 indexed address 和 32-byte uint256 data
- **THEN** 系统 MUST 精确得到 from、to 和 raw amount

#### Scenario: 日志编码畸形
- **WHEN** topic 数量不足、topic0 错误、地址 padding 非法或 data 长度不为 32 bytes
- **THEN** 当前扫描批次 MUST 失败并保持游标不推进
- **THEN** 系统 MUST 记录脱敏结构化错误且不得 panic

### Requirement: 扫描器必须支持可恢复游标和重叠重扫
系统 SHALL 按有界区块批次扫描日志，在数据库持久化扫描游标，并 MUST 重叠重扫最近区块以处理 RPC 延迟和短重组。只有完整批次成功后才能推进游标。

#### Scenario: 批次扫描成功
- **WHEN** 一个区块批次的 RPC 查询、日志解析、地址匹配和数据库写入全部成功
- **THEN** 系统 MUST 原子推进 `last_scanned_block` 到批次末端

#### Scenario: 批次中途失败
- **WHEN** RPC、解析或数据库写入在批次完成前失败
- **THEN** 系统 MUST NOT 推进该批次游标
- **THEN** 重试时已经插入的充值 MUST 由事件唯一键去重

#### Scenario: 重叠区间返回重复日志
- **WHEN** overlap 重扫再次返回已持久化日志
- **THEN** 系统 MUST 保留原充值记录
- **THEN** 系统 MUST NOT 创建重复充值或重复增加 Web3 子账户余额

### Requirement: 扫描器必须支持多实例领导租约
系统 SHALL 复用多实例领导锁，使同一 scanner key 在任意时刻只有一个实例推进游标。失去租约的实例 MUST NOT 以旧租约提交新的游标位置。

#### Scenario: 两个实例同时启动
- **WHEN** 两个应用实例竞争同一 scanner key
- **THEN** 只有持有有效领导租约的实例 MUST 执行扫描并推进游标

#### Scenario: Leader 在扫描中失去租约
- **WHEN** 当前 leader 在批次处理中失去租约
- **THEN** 该实例 MUST 放弃推进游标
- **THEN** 新 leader MUST 从数据库已提交游标继续扫描

### Requirement: 一笔交易中的多个日志必须独立处理
系统 MUST 按 RPC `logIndex` 区分同一交易中的不同 Transfer，一笔交易可以产生多条独立充值。

#### Scenario: 同一交易两次转入相同用户地址
- **WHEN** 同一 tx hash 包含两个转入相同充值地址且 log index 不同的合法 USDT0 Transfer
- **THEN** 系统 MUST 创建两条充值记录
- **THEN** 两条记录 MUST 各自经过最终确认和入账

#### Scenario: 相同 tx hash 在不同链出现
- **WHEN** 其他链出现相同 tx hash
- **THEN** `chain_id` MUST 参与事件唯一性判断
- **THEN** 非配置链事件 MUST NOT 被当前 scanner 接受

### Requirement: 系统必须使用 finalized 区块重新验证充值
系统 SHALL 仅在充值区块不高于 RPC `finalized` block 时尝试最终确认，并 MUST 重新验证 canonical block hash、成功 receipt 和目标 Transfer 日志的 Token、to、raw amount 与 log index。

#### Scenario: 充值尚未 finalized
- **WHEN** 充值 block number 高于当前 finalized block
- **THEN** 充值 MUST 保持 confirming
- **THEN** 系统 MUST NOT 增加用户余额

#### Scenario: 区块重组导致日志消失
- **WHEN** stored block hash 与 canonical block hash 不同，或 receipt 中目标日志不存在
- **THEN** 系统 MUST 将未入账充值标记为 orphaned
- **THEN** 系统 MUST NOT 增加 Web3 子账户余额

#### Scenario: finalized 验证通过
- **WHEN** canonical block、receipt status、block hash 和目标日志全部匹配
- **THEN** 系统 MUST 根据金额和用户状态把充值转换为待自动入账、低于最低金额或人工审核
