## Why

当前系统支持支付宝、微信、Stripe 等“预先创建订单，再由支付提供商回调确认”的充值方式，但没有链上充值能力。Web3 充值与现有支付订单存在根本差异：用户可以不创建订单直接向长期地址转账；一笔交易可包含多个 ERC20 日志；入账必须处理区块重组、最终性、精确整数金额、扫描游标和链上事件幂等。

直接把交易哈希塞入现有 `payment_orders` 会破坏订单生命周期语义，且现有订单金额为两位小数、余额接口使用 `float64`，不适合作为链上事实账本。因此需要新增独立的 Web3 Deposit 垂直模块，并在最终履约阶段复用现有用户余额、缓存失效和通知能力。

## What Changes

- 新增 Conflux eSpace Mainnet USDT0 充值能力，网络固定为 `chain_id=1030`，Token 固定为官方 USDT0 合约 `0xaf37E8B6C9ED7f6318979f56Fc287d76c30847ff`，小数位固定为 6。
- 新增 HD 钱包充值地址分配：业务服务只持有账户级 xpub，从 `m/44'/60'/0'/0/{index}` 派生用户地址；助记词、根私钥和子私钥不得进入主服务。
- 用户首次打开充值页面时懒分配一个长期地址；相同用户在相同网络和钱包下重复请求必须返回同一地址。
- 新增 ERC20 `Transfer` 扫描器、扫描游标、重叠重扫、区块哈希校验、RPC 链身份预检和多实例领导租约。
- 新增链上充值状态机，覆盖检测、等待最终性、待入账、人工审核、失败重试、低于最低金额和重组孤块。
- 仅在充值所在区块进入 eSpace `finalized` 后入账；入账前重新验证 canonical block、交易回执和目标日志。
- 新增精确金额路径：链上 `uint256` 保存为十进制字符串或 `NUMERIC(78,0)`，业务金额使用 `decimal.Decimal`，不得先转换为 `float64`。
- 新增余额流水及事务性入账：唯一幂等键绑定 `chain_id + tx_hash + log_index`，同一事务更新余额、`total_recharged`、流水和充值状态。
- 入账规则固定为 `1 USDT0 = 1 USD balance`，手续费为 0，最低充值为 1 USDT0，单笔大于 10,000 USDT0 进入人工审核。
- 新增用户充值配置、地址、记录和详情 API；新增管理员充值查询、人工审核、失败重试和扫描器运行态 API。
- 新增用户充值页面，明确展示网络、Token 合约、最低金额、最终性状态和误充值风险。
- 新增结构化日志、运行指标、链高度延迟、RPC 健康、充值状态堆积和余额入账失败告警。

## Capabilities

### New Capabilities

- `web3-deposit-addresses`：定义 HD 钱包配置、地址懒分配、并发幂等、地址生命周期和密钥边界。
- `conflux-usdt0-scanning`：定义 eSpace RPC 预检、ERC20 日志发现、游标、重扫、最终性和区块重组处理。
- `web3-deposit-crediting`：定义金额精度、充值状态机、余额流水、事务性入账、大额审核和失败重试。
- `web3-deposit-console`：定义用户充值页面、充值历史、管理员审核和扫描器运行态。

### Modified Capabilities

无。现有支付订单、支付回调、充值码、退款、订阅和邀请返佣行为作为兼容基线保持不变；新增模块只通过窄接口复用缓存失效和充值成功通知。

## Confirmed Product Decisions

| 项目 | 决策 |
| --- | --- |
| 网络 | Conflux eSpace Mainnet |
| Chain ID | `1030` |
| 资产 | 官方 USDT0 Token 合约 |
| Token decimals | `6` |
| 平台余额单位 | USD |
| 换算 | `1 USDT0 = 1 USD balance` |
| 手续费 | `0` |
| 最低充值 | `1 USDT0` |
| 自动入账上限 | `10,000 USDT0`，大于该金额进入人工审核 |
| 地址 | HD 钱包派生 EOA，用户长期复用 |
| 分配时机 | 首次进入充值页面时懒分配 |
| 最终性 | `finalized` 后入账 |
| 自动归集 | 首期不实现，第二期实现 |

## Non-Goals

- 不支持 Conflux Core Space、测试网、其他 EVM 网络或其他 Token。
- 不根据 Token symbol/name 自动识别资产，只接受固定 `chain_id + contract address`。
- 不实现自动归集、CFX Gas 补充、热钱包出金或自动退款。
- 不实现跨链桥接、OFT `send`、USDT0 跨链状态跟踪或 LayerZero 消息处理。
- 不实现充值订单、固定金额匹配、实时汇率、稳定币脱锚定价或累计小额充值。
- 不在 MVP 中给 Web3 充值发放邀请返佣；若产品需要，应另行定义与现有支付订单无关的幂等返佣模型。
- 不重构所有现有余额写入路径；新余额流水先覆盖 Web3 充值，并为后续统一账本预留扩展点。
- 不允许管理员通过页面修改 xpub、派生路径、Token 合约或 chain ID。

## Impact

- **后端模块**：新增 `backend/internal/web3deposit/` 垂直模块；少量修改支付路由、Wire 启停、用户余额仓储适配和通知接线。
- **数据库**：新增 `web3_deposit_addresses`、`web3_deposits`、`web3_scanner_cursors`、`balance_ledger_entries` 及索引；不把链上记录存入 `payment_orders`。
- **运行配置**：新增启用开关、RPC 端点、账户级 xpub、钱包 ID、起始区块和轮询参数；私钥不属于应用配置。
- **运行时**：新增后台扫描器和确认/入账 Worker，必须支持多实例领导租约和优雅停止。
- **前端**：新增用户充值页与管理员 Web3 充值工作台，并补充路由、API、类型、i18n 和导航。
- **安全**：新增 HD 派生测试向量、xpub 脱敏日志、RPC 固定目标、链身份校验、人工审核审计和精确金额门禁。

## Execution References

- 架构和状态机：`design.md`
- 实施顺序：`implementation-guide.md`
- 任务拆分：`tasks.md`
- 验收要求：`verification.md`
- 官方参数和目标代码基线：`source-baseline.md`
