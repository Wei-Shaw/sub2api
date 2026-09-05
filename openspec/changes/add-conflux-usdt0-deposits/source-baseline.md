# Conflux eSpace USDT0 充值基线

## 1. 基线时间

本文件在 2026-08-05 核对。实施或上线前必须再次核对官方部署页、RPC 行为和生产配置，不得只依赖本文中的静态地址。

## 2. 官方链与资产参数

| 字段 | 固定值 |
| --- | --- |
| Network | Conflux eSpace Mainnet |
| Chain ID | `1030` |
| Native currency | CFX |
| USDT0 Token | `0xaf37E8B6C9ED7f6318979f56Fc287d76c30847ff` |
| USDT0 decimals | `6` |
| USDT0 OFT | `0xC57efa1c7113D98BdA6F9f249471704Ece5dd84A` |
| Official eSpace RPC example | `https://evm.confluxrpc.com` |

充值扫描只能监听上表的 USDT0 Token 合约。OFT 合约属于跨链能力，不是本功能的用户充值 Token 地址。

官方参考：

- USDT0 deployments：<https://docs.usdt0.to/technical-documentation/deployments>
- USDT0 developer documentation：<https://docs.usdt0.to/technical-documentation/developer/>
- Conflux eSpace networks：<https://doc.confluxnetwork.org/docs/espace/network-endpoints/>
- Conflux eSpace transaction finality：<https://doc.confluxnetwork.org/docs/espace/build/transaction/>
- Conflux account/address differences：<https://doc.confluxnetwork.org/docs/general/conflux-basics/accounts/>

## 3. 目标仓库现状

### 用户余额

- `backend/ent/schema/user.go`：`balance`、`frozen_balance`、`total_recharged` 在 PostgreSQL 使用 `decimal(20,8)`，Ent 字段仍为 `float64`。
- `backend/internal/service/user_service.go`：`UserRepository` 提供 `UpdateBalance`、`AdjustBalance`、`SetBalance` 等原子余额入口，但接口参数和 `BalanceChange` 仍为 `float64`。
- `backend/internal/repository/user_repo.go`：现有余额原子写入可作为并发控制参考，但 Web3 入账需要新增 exact-decimal 事务接口，不能把 Token 金额先降级为浮点数。

### 普通支付

- `backend/ent/schema/payment_order.go`：支付订单围绕预创建订单、支付提供商和两位小数金额建模，不适合作为链上日志事实表。
- `backend/internal/service/payment_fulfillment.go`：已有支付履约租约、失败重试、充值成功通知和余额后处理逻辑，可抽取共享能力。
- `backend/internal/server/routes/payment.go`：现有用户支付 API 位于 `/api/v1/payment`，Web3 用户 API 应挂在 `/api/v1/payment/web3` 下保持导航和鉴权一致。
- `backend/internal/service/payment_config_service.go`：现有支付设置包含 `float64` 金额和提供商配置；Web3 固定链参数与 xpub 不应混入可由管理员编辑的普通支付配置。

### 运行时和依赖

- `backend/internal/service/wire.go`、`backend/cmd/server/wire.go`：现有后台服务通过 Wire 创建并在统一 cleanup 中停止；Web3 runtime 必须遵循相同生命周期。
- `backend/go.mod`：已有 `shopspring/decimal` 和 `robfig/cron`，尚无以太坊 RPC/ABI 依赖。
- 项目已有 Redis、PostgreSQL、领导锁和后台 Worker 模式，可复用领导租约、结构化日志、缓存失效和优雅停止约定。

## 4. 实施前复核

上线前必须执行并记录：

1. 从 USDT0 官方 deployments 页面核对 Token 合约和 decimals。
2. 对每个配置 RPC 调用 `eth_chainId`，结果必须为 `0x406`。
3. 调用 `eth_getCode` 验证 Token 地址存在合约代码。
4. 只读调用 `decimals()`，结果必须为 6；运行时仍使用固定配置值，不信任每笔充值动态返回。
5. 验证 RPC 支持 `eth_getBlockByNumber("finalized", false)`。
6. 记录启用时的 finalized block，作为生产 `scan_start_block`。
7. 使用独立测试钱包执行一笔最小金额以上的真实充值演练。
8. 完成 HD 种子离线备份、恢复和地址重派生演练。

## 5. 不可信输入边界

- RPC 返回值、日志、交易回执、Token metadata、交易发送方和用户提供的 tx hash 都是不可信输入。
- 只有固定 `chain_id + token_contract + Transfer topic + assigned to-address` 同时匹配时才创建候选充值。
- 前端显示的 symbol、网络名称和二维码不能参与后端资产识别。
- 管理员人工审核只能批准已达到 finalized 且重新验证成功的充值，不能绕过链上验证直接按请求金额入账。
