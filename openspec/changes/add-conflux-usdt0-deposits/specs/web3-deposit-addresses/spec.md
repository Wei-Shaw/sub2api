## ADDED Requirements

### Requirement: 系统必须使用专用 HD 钱包为用户派生充值地址
系统 SHALL 使用专用于 Conflux eSpace 充值的 HD 钱包账户级 xpub，从 `m/44'/60'/0'/0/{index}` 派生用户 EOA 地址。主业务服务 MUST NOT 持有、加载、记录或返回助记词、根私钥、账户私钥或子私钥。

#### Scenario: 为用户派生首个地址
- **WHEN** 活跃用户首次请求创建充值地址
- **THEN** 系统 MUST 从配置钱包的账户级 xpub 派生 `/0/{index}` 子公钥
- **THEN** 系统 MUST 将其转换为合法的 20-byte EVM 地址
- **THEN** 系统 MUST 持久化 `wallet_id`、`derivation_index`、checksum address 和 lowercase normalized address

#### Scenario: 主服务配置包含私有扩展密钥
- **WHEN** 配置值可被解析为 xprv 或其他私有扩展密钥
- **THEN** Web3 Deposit runtime MUST fail closed
- **THEN** 系统 MUST NOT 分配充值地址或启动自动入账

### Requirement: 地址分配必须并发幂等
系统 SHALL 为相同 `(user_id, chain_id, wallet_id)` 最多分配一个充值地址，并 MUST 使用数据库事务、原子 derivation index 分配和唯一约束保证多实例并发安全。

#### Scenario: 同一用户并发创建地址
- **WHEN** 同一用户并发提交多个创建地址请求
- **THEN** 所有成功响应 MUST 返回同一个地址记录
- **THEN** 数据库 MUST 只存在一个用户地址映射

#### Scenario: 派生后插入失败
- **WHEN** 系统已经消耗一个 derivation index 但地址记录插入失败
- **THEN** 系统 MAY 留下 index 空洞
- **THEN** 系统 MUST NOT 回退或重新分配该 index 给其他用户

### Requirement: 钱包身份必须不可静默替换
系统 SHALL 使用稳定 `wallet_id` 和 xpub fingerprint 绑定一套 HD 钱包。已经分配过地址的 `wallet_id` MUST NOT 静默指向不同 xpub。

#### Scenario: 运行配置 fingerprint 与数据库不一致
- **WHEN** 启动时配置 xpub 的 fingerprint 与该 `wallet_id` 已记录 fingerprint 不一致
- **THEN** 系统 MUST 拒绝分配新地址
- **THEN** 系统 MUST 将 runtime 标记为 unhealthy
- **THEN** 历史地址和充值记录 MUST 仍可查询

#### Scenario: 使用新钱包轮换地址
- **WHEN** 运维需要切换到另一套 HD 钱包
- **THEN** 系统 MUST 使用新的 `wallet_id`
- **THEN** 旧 wallet ID 派生的所有历史地址 MUST 继续被扫描

### Requirement: 用户地址必须懒分配并长期复用
系统 SHALL 在用户首次请求 Web3 充值地址时懒分配地址。相同用户后续请求 MUST 返回同一地址，MVP MUST NOT 自动轮换活跃地址。

#### Scenario: 查询尚未分配的地址
- **WHEN** 用户调用只读地址查询且尚未分配地址
- **THEN** 系统 MUST 返回明确的未分配状态
- **THEN** 查询 MUST NOT 消耗 derivation index 或创建数据库记录

#### Scenario: 重复进入充值页面
- **WHEN** 已有地址的用户再次进入充值页面或重复调用创建接口
- **THEN** 系统 MUST 返回原地址
- **THEN** 系统 MUST NOT 派生新地址

### Requirement: 历史地址必须永久保留充值归属
系统 MUST 保留所有已分配地址与用户、钱包和 derivation index 的映射，即使用户被禁用、软删除、地址被停用、钱包被轮换或功能被关闭，也 MUST NOT 删除或重用历史地址。

#### Scenario: 用户被禁用后收到充值
- **WHEN** 已禁用或软删除用户的历史地址收到合法 USDT0 Transfer
- **THEN** 扫描器 MUST 识别并持久化该充值
- **THEN** 系统 MUST 将其送入人工审核而不是自动入账或静默丢弃

#### Scenario: 地址被标记为 disabled
- **WHEN** 地址状态被设置为 disabled
- **THEN** 系统 MUST 禁止将其作为新的展示地址
- **THEN** 系统 MUST 继续识别转入该地址的历史兼容充值

### Requirement: 地址响应不得泄露钱包派生信息
用户和管理员 API SHALL 只暴露完成业务操作所需的地址信息，并 MUST NOT 返回完整 xpub、derivation index、账户路径、子公钥或任何私钥材料。

#### Scenario: 用户读取充值地址
- **WHEN** 用户成功读取其充值地址
- **THEN** 响应 MAY 包含地址、网络、Token 合约和分配时间
- **THEN** 响应 MUST NOT 包含 wallet fingerprint、xpub 或 derivation index

#### Scenario: 地址分配失败被记录
- **WHEN** 地址派生或写入失败
- **THEN** 日志 MUST 只使用 wallet ID、内部错误码和必要的非敏感 index
- **THEN** 日志 MUST NOT 输出完整 xpub 或私钥材料
