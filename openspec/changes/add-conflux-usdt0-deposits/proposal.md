## Why

当前系统支持支付宝、微信、Stripe 等“预先创建订单，再由支付提供商回调确认”的充值方式，但没有链上充值能力。Web3 充值与现有支付订单存在根本差异：用户可以不创建订单直接向长期地址转账；一笔交易可包含多个 ERC20 日志；入账必须处理区块重组、最终性、精确整数金额、扫描游标和链上事件幂等。

直接把交易哈希塞入现有 `payment_orders` 会破坏订单生命周期语义，且现有订单金额为两位小数、余额接口使用 `float64`，不适合作为链上事实账本。因此需要新增独立的 Web3 Deposit 垂直模块，并在最终履约阶段复用现有用户余额、缓存失效和通知能力。

## Core Asset Assumptions

以下假设是当前 Web3 充值计价和余额模型成立的前提，不是普通运维配置偏好：

1. **只支持美元稳定币。** 受支持 Token 必须是经过产品和运维确认的 USD stablecoin，系统按 `1 Token = 1 USD` 记入内部 `usdt` Web3 余额。当前实现不读取市场价格，不处理汇率，也不处理稳定币脱锚。
2. **每个网络默认只支持一种充值 Token。** 当前经过实现和验收覆盖的部署模型是“多个网络、每个网络一个 Token”。配置中的 `assets` 映射只保留未来扩展空间，不代表同一网络多 Token 已获支持或已完成余额隔离验证。
3. **扩展资产必须先扩展产品模型。** 非美元稳定币、波动资产或同一网络的第二种 Token 不能仅靠新增配置上线；必须先明确内部资产键、计价规则、余额聚合/隔离、脱锚策略和迁移方案，并更新 Spec 与测试。

## What Changes

- 新增可配置的 Conflux eSpace 网络充值能力；默认主网为 `chain_id=1030`，默认 Token 为官方 USDT0 合约 `0xaf37E8B6C9ED7f6318979f56Fc287d76c30847ff`。每个启用网络只配置一种经过确认的 6 位 USD 稳定币。
- 新增 HD 钱包充值地址分配：业务服务只持有账户级 xpub，从 `m/44'/60'/0'/0/{index}` 派生用户地址；助记词、根私钥和子私钥不得进入主服务。
- 用户首次打开充值页面时懒分配一个长期地址；相同用户在相同网络和钱包下重复请求必须返回同一地址。
- 新增 ERC20 `Transfer` 扫描器、扫描游标、重叠重扫、区块哈希校验、RPC 链身份预检和多实例领导租约。
- 新增链上充值状态机，覆盖检测、等待最终性、待入账、人工审核、失败重试、低于最低金额和重组孤块。
- 仅在充值所在区块进入 eSpace `finalized` 后入账；入账前重新验证 canonical block、交易回执和目标日志。
- 新增精确金额路径：链上 `uint256` 保存为十进制字符串或 `NUMERIC(78,0)`，业务金额使用 `decimal.Decimal`，不得先转换为 `float64`。
- 新增 Web3 子账户：`web3_deposits` 记录链上充值事实，`web3_user_balances` 保存用户按资产聚合的可用余额，`web3_balance_transfers` 记录 Web3 余额向 `users.balance` 的幂等划转事实。
- 入账规则固定为 `1 USDT0 = 1 USD balance`，手续费为 0，最低充值为 1 USDT0，单笔大于 10,000 USDT0 进入人工审核。
- 新增用户充值配置、地址、记录和详情 API；新增管理员充值查询、人工审核、失败重试和扫描器运行态 API。
- 新增用户充值页面，明确展示网络、Token 合约、最低金额、最终性状态和误充值风险。
- 新增结构化日志、运行指标、链高度延迟、RPC 健康、充值状态堆积和余额入账失败告警。

## Capabilities

### New Capabilities

- `web3-deposit-addresses`：定义 HD 钱包配置、地址懒分配、并发幂等、地址生命周期和密钥边界。
- `conflux-usdt0-scanning`：定义 eSpace RPC 预检、ERC20 日志发现、游标、重扫、最终性和区块重组处理。
- `web3-deposit-crediting`：定义金额精度、充值状态机、Web3 子账户、向主余额划转、大额审核和失败重试。
- `web3-deposit-console`：定义用户充值页面、充值历史、管理员审核和扫描器运行态。

### Modified Capabilities

无。现有支付订单、支付回调、充值码、退款、订阅和邀请返佣行为作为兼容基线保持不变；新增模块只通过窄接口复用缓存失效和充值成功通知。

## Confirmed Product Decisions

| 项目 | 决策 |
| --- | --- |
| 网络 | 可配置的 Conflux eSpace 网络；默认 Mainnet |
| Chain ID | 每个网络独立配置；默认 `1030` |
| 资产 | 每个网络一种经过确认的 USD 稳定币；默认官方 USDT0 |
| Token decimals | `6` |
| 平台余额单位 | USD |
| 换算 | `1 USDT0 = 1 USD balance` |
| 受支持资产类型 | 仅经过确认的美元稳定币 |
| 单网络 Token 数量 | 默认且已验证为 1 种 |
| 手续费 | `0` |
| 最低充值 | `1 USDT0` |
| 自动入账上限 | `10,000 USDT0`，大于该金额进入人工审核 |
| 地址 | HD 钱包派生 EOA，用户长期复用 |
| 分配时机 | 首次进入充值页面时懒分配 |
| 最终性 | `finalized` 后入账 |
| 自动归集 | 首期不实现，第二期实现 |

## Non-Goals

- 不支持 Conflux Core Space、未配置网络、非美元稳定币或波动 Token。
- 不根据 Token symbol/name 自动识别资产，只接受固定 `chain_id + contract address`。
- 不实现自动归集、CFX Gas 补充、热钱包出金或自动退款。
- 不实现跨链桥接、OFT `send`、USDT0 跨链状态跟踪或 LayerZero 消息处理。
- 不实现充值订单、固定金额匹配、实时汇率、稳定币脱锚定价或累计小额充值。
- 不支持非美元稳定币、波动资产，或未经独立设计和验收的同一网络多 Token 充值。
- 不在 MVP 中给 Web3 充值发放邀请返佣；若产品需要，应另行定义与现有支付订单无关的幂等返佣模型。
- 不重构所有现有余额写入路径；Web3 充值先进入独立子账户，只有显式划转才修改现有主余额。
- 不允许管理员通过页面修改 xpub、派生路径、Token 合约或 chain ID。

## Impact

- **后端模块**：新增 `backend/internal/web3deposit/` 垂直模块；少量修改支付路由、Wire 启停、用户余额仓储适配和通知接线。
- **数据库**：新增 `web3_deposit_wallets`、`web3_deposit_addresses`、`web3_deposits`、`web3_scanner_cursors`、`web3_user_balances`、`web3_balance_transfers`、`web3_rescan_jobs` 及索引；不把链上记录存入 `payment_orders`。
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
