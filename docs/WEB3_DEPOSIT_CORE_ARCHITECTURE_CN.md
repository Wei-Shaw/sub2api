# Web3 充值核心实现说明

本文说明当前 Web3 充值功能的两个核心部分：

1. 为用户生成并长期绑定 EVM 充值地址。
2. 扫描链上充值、确认最终性并将有效充值计入用户 Web3 余额。

本文侧重代码实现和数据一致性。部署、开关、故障处理和人工审核操作参见 [WEB3_DEPOSIT_RUNBOOK.md](./WEB3_DEPOSIT_RUNBOOK.md)。

## 1. 当前产品边界

当前实现基于以下明确假设：

- 只支持产品和运营批准的美元稳定币。
- Token 按固定 `1 Token = 1 USD` 计入内部 `usdt` Web3 余额，不包含价格预言机、汇率换算或脱锚处理。
- 支持配置多个 EVM 网络，但当前经过验证的模型是每个网络只支持一种充值 Token。
- 所有启用网络必须使用同一个 `wallet_id`，因此同一用户在所有启用网络上使用同一个 EVM 充值地址。
- 当前充值金额转换要求 Token 精度为 6 位，符合现有 USDT/USDT0 模型。
- 自动入账先增加 `web3_user_balances`，不会直接增加主账户 `users.balance`；用户需要再执行一次余额转入操作。
- 充值地址的 Gas 补充、交易签名和资金归集不属于本项目职责，将由独立的 Sweep/Custody 项目实现。

配置中的 `assets` 映射是为数据结构扩展保留的，并不代表当前已经支持同一网络配置多个 Token。增加非美元资产、波动资产或同一网络的第二种 Token，需要单独设计、实现和验收。

本项目以“充值事实已确认并准确计入用户 Web3 余额”为链路终点。独立归集项目应只消费地址和链上事实，不应直接修改本项目的充值状态或用户余额。

## 2. 总体架构

完整流程如下：

```text
用户进入充值页面
    |
    v
获取或创建长期充值地址
    |
    v
用户向该地址发送指定 Token
    |
    v
Scanner 扫描 ERC-20 Transfer 日志
    |
    v
匹配系统充值地址并记录 detected 充值
    |
    v
Finalizer 等待 finalized 并重新核验链上事实
    |
    +--> below_minimum / manual_review / orphaned / failed
    |
    v
ready_to_credit
    |
    v
Credit Worker 原子增加 Web3 余额
    |
    v
credited
```

这两个核心部分通过 `web3_deposit_addresses` 关联：地址生成模块记录地址与用户的归属关系，扫描模块用该关系把链上 Transfer 事件归属到具体用户。

## 3. 核心一：充值地址生成

### 3.1 HD Wallet 模型

地址生成采用分层确定性钱包模型：

```text
离线助记词或私钥
    |
    v
账户级 xpub: m/44'/60'/{account}'
    |
    v
外部地址分支: /0
    |
    v
用户派生索引: /{derivation_index}
```

每个用户地址的完整路径为：

```text
m/44'/60'/{account}'/0/{derivation_index}
```

应用只配置账户级 `account_xpub`，不保存助记词或私钥，因此应用可以生成和识别地址，但不能签名或转出链上资产。私钥和资金归集能力必须保留在独立的安全钱包环境中。

地址派生实现在 `backend/internal/web3deposit/address_deriver.go`。

### 3.2 用户请求入口

用户充值页面加载公开配置后，在功能可用时自动调用：

```http
POST /api/v1/payment/web3/address
```

该接口统一执行“获取或创建”，用户不需要单独点击创建按钮，多次请求具有幂等语义。

接口首先检查：

- 用户已经登录。
- `web3_deposit.enabled=true`。
- `user_entry_enabled=true`。
- 至少有一个启用且运行健康的充值网络。
- 启用网络使用同一个 `wallet_id`。
- 对应钱包配置包含有效的 `account_path` 和 `account_xpub`。

入口实现在：

- `frontend/src/views/user/Web3DepositAddressView.vue`
- `frontend/src/api/web3Deposit.ts`
- `backend/internal/handler/web3_deposit_handler.go`

### 3.3 获取或创建地址

`AddressAllocator.GetOrCreate` 按以下顺序工作：

1. 使用 `(user_id, wallet_id)` 查询已有地址。
2. 如果已有地址为 `active`，直接返回，不消耗新的派生索引。
3. 如果已有地址为 `disabled`，拒绝返回，也不会静默创建替代地址。
4. 如果地址不存在，校验钱包身份。
5. 原子预留一个新的派生索引。
6. 使用 `account_xpub` 和索引派生 EVM 地址。
7. 将地址与用户归属关系写入数据库。

核心实现在 `backend/internal/web3deposit/address_allocator.go`。

### 3.4 钱包身份固定

系统第一次使用某个 `wallet_id` 时，会在 `web3_deposit_wallets` 保存：

```text
wallet_id
account_path
SHA-256(account_xpub)
next_derivation_index
status
```

后续启动 Scanner 或分配地址时，当前配置的 `account_path` 和 xpub 指纹必须与数据库完全一致，钱包状态也必须为 `active`。如果运维人员误将已有 `wallet_id` 指向另一个 xpub 或路径，系统会拒绝新地址分配，并将相关运行时标记为不健康。

该约束防止新生成地址来自另一个钱包，而系统仍把它们当作原钱包地址。实现位于 `backend/internal/web3deposit/wallet_verifier.go`。

### 3.5 原子预留派生索引

每个钱包维护全局递增的 `next_derivation_index`。预留索引时使用带旧值条件的数据库更新，相当于 Compare-And-Swap：

```sql
UPDATE web3_deposit_wallets
SET next_derivation_index = next_derivation_index + 1
WHERE wallet_id = :wallet_id
  AND account_path = :account_path
  AND xpub_fingerprint = :fingerprint
  AND status = 'active'
  AND next_derivation_index = :previous_index;
```

只有一个并发请求能获得当前索引，其余请求重新读取并重试。索引范围为：

```text
0 <= derivation_index < 2^31
```

实现位于 `backend/internal/repository/web3_deposit_wallet_repo.go`。

### 3.6 EVM 地址计算

取得索引后，系统执行：

```text
account_xpub
    -> derive(0)
    -> derive(derivation_index)
    -> secp256k1 子公钥
    -> Keccak-256(未压缩公钥正文)
    -> 取最后 20 字节
    -> EVM 地址
```

数据库同时保存：

- `address`：EIP-55 checksum 格式，用于展示。
- `normalized_address`：全小写格式，用于稳定匹配链上事件。

### 3.7 数据库唯一性和并发兜底

`web3_deposit_addresses` 包含三个关键唯一约束：

```text
(user_id, wallet_id)           一个用户在一个钱包下只有一个地址
(wallet_id, derivation_index)  一个派生索引只能使用一次
normalized_address             一个地址只能归属一个用户
```

如果同一用户并发发起多个请求，先完成的请求创建地址；其余请求遇到唯一约束冲突后重新查询并返回已创建的地址。由数据库约束承担最终一致性保证。

Schema 和仓储实现位于：

- `backend/ent/schema/web3_deposit_address.go`
- `backend/internal/repository/web3_deposit_address_repo.go`

### 3.8 多网络地址语义

当前地址归属键是：

```text
user_id + wallet_id
```

而不是：

```text
user_id + network_id
```

由于所有启用网络绑定同一个 EVM 钱包，同一个用户地址可以在 Conflux eSpace 等不同 EVM 网络上使用。切换网络不会改变充值地址，只会改变 Chain ID、Token 合约和负责扫描的 Runtime。

## 4. 核心二：充值扫描和入账

### 4.1 Scanner Runtime 隔离

系统为每个启用的 `(network_key, asset_key)` 创建独立 Runtime，Scanner Key 格式为：

```text
network_key:asset_key
```

每个 Runtime 独立维护：

- Chain ID 和 Token 合约。
- RPC Endpoint 池。
- `scan_start_block`。
- `last_scanned_block`。
- `last_finalized_block`。
- 轮询间隔、扫描批量和重叠区块数。
- Leader 租约和运行状态。

组装入口位于 `backend/internal/web3deposit/scanner_runtime_provider.go`。

### 4.2 多实例 Leader 租约

同一个 Scanner Key 同时只允许一个实例扫描。实例通过数据库租约进入以下状态：

- `leader`：持有租约并扫描。
- `standby`：等待租约或准备接管。
- `unhealthy`：钱包、网络、RPC 或运行时校验失败。
- `disabled`：扫描功能关闭。

Leader 定期续租。进程崩溃后，其他实例可在租约过期后接管。扫描批次和游标更新都必须携带有效 `lease_token`，从而阻止丢失租约的旧实例继续提交结果。

实现位于：

- `backend/internal/web3deposit/scanner_runtime.go`
- `backend/internal/repository/web3_scanner_cursor_repo.go`

### 4.3 扫描区间和重叠扫描

每轮扫描读取持久化游标并调用 `eth_blockNumber` 获取最新高度，然后计算本批区间：

```text
from = last_scanned_block - overlap_blocks
to   = min(from + block_batch_size - 1, latest_block)
```

实际计算会保护 `scan_start_block` 下界；游标边界块也会再次扫描。重叠扫描降低 RPC 暂时漏日志和边界区块变化造成漏单的风险。重复事件通过唯一约束去重。

实现位于 `backend/internal/web3deposit/scanner.go`。

### 4.4 RPC 拉取 ERC-20 Transfer

Scanner 使用 `eth_getLogs` 查询指定 Token 合约的：

```solidity
Transfer(address,address,uint256)
```

RPC 层支持：

- 多 Endpoint 轮换和故障转移。
- 单次请求超时。
- 失败 Endpoint 冷却。
- RPC 报告区间过大时自动缩小查询范围。

日志解析会验证合约地址、Transfer 签名、Topic 数量、地址编码、金额长度以及 `removed` 标志，并保留区块号、区块 Hash、交易 Hash和 Log Index。

实现位于：

- `backend/internal/web3deposit/conflux_rpc_pool.go`
- `backend/internal/web3deposit/transfer_log_fetcher.go`
- `backend/internal/web3deposit/erc20_transfer_parser.go`

### 4.5 充值地址匹配

系统将本批所有 Transfer 的 `to` 地址转成小写，分批查询 `web3_deposit_addresses.normalized_address`。匹配成功后得到：

```text
deposit_address_id
user_id
```

此处匹配历史地址，不只匹配当前 `active` 地址。地址和用户是否仍满足自动入账条件，会在最终确认阶段重新判断。

实现位于 `backend/internal/web3deposit/recipient_matcher.go`。

### 4.6 记录 detected 充值

匹配成功的事件写入 `web3_deposits`，包含完整链上事实：

```text
user_id
deposit_address_id
chain_id
token_contract
tx_hash
log_index
block_number
block_hash
from_address
to_address
raw_amount
token_amount
status = detected
```

链上事件的唯一键是：

```text
(chain_id, tx_hash, log_index)
```

不能只用交易 Hash，因为一次交易可以产生多个 Transfer 日志。重复扫描使用 `ON CONFLICT DO NOTHING` 返回已有充值，不会重复创建记录。

充值落库和推进 `last_scanned_block` 在同一个数据库事务中完成。任何一步失败都会回滚，因此不会出现“游标已经越过区块，但充值记录没有保存”。

实现位于：

- `backend/internal/web3deposit/deposit_event_persister.go`
- `backend/internal/repository/web3_deposit_repo.go`
- `backend/internal/repository/web3_scanner_batch_repo.go`

### 4.7 Finalizer 最终性确认

发现充值不代表可以入账。Finalizer 通过以下 RPC 获取链的最终区块：

```text
eth_getBlockByNumber("finalized", false)
```

它只处理不超过 `finalized` 高度且不超过 `last_scanned_block` 的充值。对于每笔候选记录，Finalizer 会重新验证：

- 原区块仍然存在。
- canonical block hash 与扫描时一致。
- Transaction Receipt 存在且执行成功。
- Receipt 仍属于同一个区块。
- 对应 `log_index` 仍存在。
- Token 合约、收款地址和原始金额与已保存事实一致。

任何一项不一致都会将充值标记为 `orphaned`，用于处理区块重组或非 canonical 事件。

实现位于：

- `backend/internal/web3deposit/finalizer.go`
- `backend/internal/web3deposit/canonical_deposit_verifier.go`

### 4.8 最终确认后的分类

通过 canonical 验证后，充值按金额分类：

| 条件 | 状态 | 处理方式 |
| --- | --- | --- |
| 金额小于 `minimum_deposit` | `below_minimum` | 记录但不自动入账 |
| 金额不超过 `auto_credit_limit` | `ready_to_credit` | 进入自动入账队列 |
| 金额超过 `auto_credit_limit` | `manual_review` | 等待管理员审核 |
| 金额超过平台余额字段范围 | `failed` | 拒绝自动处理 |
| canonical 验证失败 | `orphaned` | 不入账 |

进入 `ready_to_credit` 前还会检查：

- 用户存在、未删除且状态为 `active`。
- 充值地址存在且状态为 `active`。
- 地址仍属于充值记录中的用户。
- 地址内容仍与链上收款地址一致。

不满足条件的充值转为 `manual_review`。充值分类结果和 `last_finalized_block` 在同一事务中提交。

实现位于：

- `backend/internal/web3deposit/finalized_deposit_classifier.go`
- `backend/internal/repository/web3_deposit_repo.go`
- `backend/internal/repository/web3_finalizer_batch_repo.go`

### 4.9 Credit Worker 领取和重试

当 `credit_enabled=true` 时，独立 Credit Worker 定期领取 `ready_to_credit` 充值：

```text
ready_to_credit -> crediting
```

领取使用 PostgreSQL `FOR UPDATE SKIP LOCKED`，多个 Worker 可以并行运行，但同一笔充值只能被一个 Worker 领取。领取后设置处理租约和递增的 `retry_count`：

- Worker 正常完成后状态变为 `credited`。
- Worker 崩溃后，`crediting` 租约过期即可重新领取。
- 临时失败会退回 `ready_to_credit`，按指数退避延迟重试。
- `retry_count` 同时充当 Claim Version，旧 Worker 不能提交已被新 Worker 接管的任务。

实现位于：

- `backend/internal/web3deposit/credit_worker.go`
- `backend/internal/repository/web3_credit_job_repo.go`

### 4.10 原子增加 Web3 余额

真正入账在一个数据库事务中完成：

1. 锁定充值记录和用户记录。
2. 验证充值仍由当前 Claim Version 持有。
3. 创建或锁定用户的 `web3_user_balances`。
4. 增加 `available_amount` 和 `total_deposited`。
5. 将充值状态改为 `credited`。
6. 写入 `credited_amount` 和 `credited_at`。
7. 提交事务。

因此不会出现余额增加但充值状态未更新，或充值显示成功但余额未增加。已经是 `credited` 的充值再次执行时只返回幂等结果，不会再次增加余额。

实现位于 `backend/internal/repository/web3_accounting_repo.go`。

## 5. 充值状态流

正常流程：

```text
detected
    -> ready_to_credit
    -> crediting
    -> credited
```

其他分支：

```text
detected/confirming -> below_minimum
detected/confirming -> manual_review
detected/confirming -> orphaned
detected/confirming -> failed

manual_review -> 管理员批准 -> ready_to_credit
manual_review -> 管理员忽略 -> ignored

crediting -> 临时失败 -> ready_to_credit -> 重试
```

状态及允许的状态转换定义在 `backend/internal/web3deposit/status.go`。

## 6. 一致性和幂等性总结

| 风险 | 当前实现的保护方式 |
| --- | --- |
| 同一用户重复申请地址 | `(user_id, wallet_id)` 唯一约束和 Get-or-Create |
| 不同用户获得相同派生索引 | 钱包索引 Compare-And-Swap 和唯一约束 |
| 配置错误导致切换钱包 | `account_path` 和 xpub 指纹固定 |
| 多实例重复扫描 | Scanner Leader 租约和 Lease Token fencing |
| RPC 漏掉边界日志 | 持久化游标和重叠扫描 |
| 同一事件重复落库 | `(chain_id, tx_hash, log_index)` 唯一约束 |
| 游标推进但充值未保存 | 充值记录和扫描游标同事务提交 |
| 区块重组 | finalized 高度和 canonical Receipt 二次核验 |
| 多 Worker 重复领取 | `FOR UPDATE SKIP LOCKED` 和 Claim Version |
| 重复增加用户余额 | 充值状态、行锁和余额更新同事务提交 |
| Worker 或服务中途退出 | 持久化游标、过期租约接管和退避重试 |

## 7. 核心结论

充值地址生成模块使用账户级 xpub 和数据库原子递增索引，为每个用户派生一个长期、唯一、可恢复的 EVM 地址，并通过钱包指纹和数据库唯一约束固定地址归属。

充值扫描模块使用持久化游标和 Leader 租约扫描指定 Token 的 Transfer 日志，通过地址归属匹配用户，再以链的 finalized 状态和 canonical Receipt 重新验证事件，最后由可恢复、幂等、事务化的 Credit Worker 将有效充值计入用户 Web3 余额。
