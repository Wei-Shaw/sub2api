## Context

### 当前系统

当前支付路径以 `payment_orders` 为中心：用户创建订单，支付提供商回调，订单进入 `PAID/RECHARGING/COMPLETED`，余额充值通过充值码兑换完成。该模型假设存在内部订单号、期望金额、支付过期时间和提供商交易号。

链上充值没有这些前提。用户可随时向长期地址发送任意金额；系统事实来源是指定 Token 合约产生的 ERC20 `Transfer` 日志；最终入账标识必须包含 `chain_id + tx_hash + log_index`；发现与最终确认之间可能发生重组。因此本能力使用独立领域模型，只在“增加 USD 余额”这一履约边界与现有系统汇合。

### 固定产品参数

```text
network            = Conflux eSpace Mainnet
chain_id           = 1030
token_contract     = 0xaf37E8B6C9ED7f6318979f56Fc287d76c30847ff
token_decimals     = 6
minimum_deposit    = 1.000000 USDT0
auto_credit_limit  = 10000.000000 USDT0
credit_rate        = 1 USDT0 : 1 USD balance
fee_rate           = 0
credit_finality    = finalized
address_scheme     = m/44'/60'/0'/0/{index}
sweeping           = disabled in MVP
```

### 参与边界

```text
User/API
  -> AddressService
      -> HD Public Deriver
      -> DepositAddressRepository

Conflux eSpace RPC
  -> ScannerRuntime
      -> LogParser
      -> DepositRepository
      -> ScannerCursorRepository
  -> Finalizer
      -> CanonicalVerifier
      -> CreditService
          -> BalanceLedgerRepository
          -> User exact-decimal update
          -> Billing cache invalidation
          -> Notification service
```

## Goals / Non-Goals

### Goals

- 每个用户稳定获得一个可恢复、可审计的 eSpace EOA 充值地址。
- 扫描指定 USDT0 合约全部 `Transfer` 日志，并只接受转入已分配地址的日志。
- 在多实例、重复扫描、进程崩溃和 RPC 重试下保持最多一次余额入账。
- 链上金额从 RPC 到 PostgreSQL 再到余额更新全程保持十进制精度。
- 只有 finalized 且重新验证通过的事件才能自动增加余额。
- 用户能够看到充值从检测到到账的状态；管理员能够处理大额和失败事件。
- 主应用无法签名或移动用户充值地址中的资金。

### Non-Goals

- 自动归集、Gas 补充、出金、退款、跨链桥和多资产抽象。
- 在 MVP 中统一改造全部支付、兑换码、管理员调账和计费余额流水。
- 允许用户绑定自有钱包作为充值地址，或允许管理员导入任意地址。
- 在未达到 finalized 时基于风险评分提前放款。

## Decisions

### 1. 使用独立 `web3deposit` 垂直模块

新增目录建议：

```text
backend/internal/web3deposit/
  config.go
  domain.go
  amount.go
  address_deriver.go
  address_service.go
  rpc_client.go
  log_parser.go
  scanner.go
  finalizer.go
  credit_service.go
  repository.go
  postgres_repository.go
  handler.go
  admin_handler.go
  runtime.go
  metrics.go
  wire.go
```

支付模块可以调用共享的余额后处理接口，但 `web3deposit` 不依赖 `PaymentOrder`、支付 provider registry 或充值码服务。

### 2. 固定资产身份，不信任 symbol

资产唯一身份为：

```text
(chain_id=1030, token_contract=lowercase(0xaf37...47ff))
```

扫描器只请求该 Token 合约的日志，并只解析 `Transfer(address,address,uint256)`。`symbol()`、`name()`、前端文案和交易发送方都不参与资产判断。

USDT0 OFT 合约不得配置为充值 Token。运行时启动预检必须检查 chain ID、Token bytecode、固定 decimals 和 finalized block tag。

### 3. EVM 网络共享账户级 xpub 和用户充值地址

离线环境从助记词派生一套 EVM 充值账户：

```text
m/44'/60'/0'
```

只把该节点的账户级 xpub 提供给主服务。主服务继续派生非 hardened 路径：

```text
/0/{index}
```

得到最终用户路径：

```text
m/44'/60'/0'/0/{index}
```

约束：

- 配置包含 `wallet_id`、`xpub`、`account_path` 和可选公钥指纹。
- 引用同一 `wallet_id` 的 EVM 网络共享同一套用户地址；地址分配身份不包含 `chain_id`。
- `wallet_id` 一旦分配过地址，不得指向另一套 xpub。
- 日志、错误、指标和管理 API 不得输出完整 xpub。
- 主服务依赖中不得出现助记词导入、私钥导出或交易签名入口。
- 第二期归集 signer 根据 `wallet_id + derivation_index` 在隔离环境签名，不从主数据库读取私钥。

### 4. 地址采用懒分配和永久监听

`POST /api/v1/payment/web3/address` 执行 get-or-create：

1. 查询 `(user_id, wallet_id)` 是否已有地址。
2. 已有则直接返回。
3. 没有则在事务内锁定钱包分配器行，读取并递增 `next_derivation_index`。
4. 在事务外或事务内通过纯函数派生地址；实现必须保证失败时不会把同一 index 分配给两个用户。
5. 插入地址；唯一冲突时重新读取该用户现有地址。

允许 index 出现空洞，不允许 index 被重用。删除用户、禁用地址或更换活跃钱包后，历史地址仍保留并继续参与扫描匹配。

### 5. 地址表保存规范化值和钱包身份

建议 SQL 结构：

```sql
CREATE TABLE web3_deposit_addresses (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT NOT NULL REFERENCES users(id),
    wallet_id           VARCHAR(64) NOT NULL,
    derivation_index    BIGINT NOT NULL CHECK (derivation_index >= 0),
    address             VARCHAR(42) NOT NULL,
    normalized_address  VARCHAR(42) NOT NULL,
    status              VARCHAR(20) NOT NULL DEFAULT 'active',
    allocated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    disabled_at         TIMESTAMPTZ,
    last_deposit_at     TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT web3_deposit_addresses_status_check
      CHECK (status IN ('active', 'disabled')),
    CONSTRAINT web3_deposit_addresses_user_wallet_uniq
      UNIQUE (user_id, wallet_id),
    CONSTRAINT web3_deposit_addresses_index_uniq
      UNIQUE (wallet_id, derivation_index),
    CONSTRAINT web3_deposit_addresses_address_uniq
      UNIQUE (normalized_address)
);
```

地址本身不绑定网络。每个网络通过配置的 `wallet_id` 复用该钱包下的用户地址，
而充值事件仍使用 `chain_id + tx_hash + log_index` 区分不同链上的事实。

另建钱包分配器表或等价原子 sequence：

```sql
CREATE TABLE web3_deposit_wallets (
    wallet_id              VARCHAR(64) PRIMARY KEY,
    account_path           VARCHAR(64) NOT NULL,
    xpub_fingerprint       VARCHAR(64) NOT NULL,
    next_derivation_index  BIGINT NOT NULL DEFAULT 0,
    status                 VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

完整 xpub 来自运行配置，不写入普通业务表。数据库指纹用于检测 `wallet_id` 与运行配置不一致。

### 6. 链上原始金额不得使用浮点数

RPC 日志中的 `uint256` 先解析为 `big.Int`，持久化为 `NUMERIC(78,0)` 或规范十进制字符串。USDT0 展示金额使用：

```go
tokenAmount := decimal.NewFromBigInt(rawAmount, -6)
```

禁止：

```go
float64(rawAmount.Int64()) / 1_000_000
```

`credited_amount` 使用与用户余额兼容的 `DECIMAL(20,8)`。入账前检查：

- `raw_amount > 0`。
- 转换后不超过 `DECIMAL(20,8)` 可表示范围。
- 小数位不超过 USDT0 固定 6 位。
- `credited_amount == token_amount`，不做汇率和手续费计算。

### 7. 一条 ERC20 日志对应一条充值事实

建议 SQL 结构：

```sql
CREATE TABLE web3_deposits (
    id                  BIGSERIAL PRIMARY KEY,
    user_id             BIGINT NOT NULL REFERENCES users(id),
    deposit_address_id  BIGINT NOT NULL REFERENCES web3_deposit_addresses(id),
    chain_id            BIGINT NOT NULL,
    token_contract      VARCHAR(42) NOT NULL,
    tx_hash             VARCHAR(66) NOT NULL,
    log_index           BIGINT NOT NULL CHECK (log_index >= 0),
    block_number        BIGINT NOT NULL CHECK (block_number >= 0),
    block_hash          VARCHAR(66) NOT NULL,
    from_address        VARCHAR(42) NOT NULL,
    to_address          VARCHAR(42) NOT NULL,
    raw_amount          NUMERIC(78,0) NOT NULL CHECK (raw_amount > 0),
    token_decimals      SMALLINT NOT NULL,
    token_amount        NUMERIC(38,18) NOT NULL,
    credited_amount     DECIMAL(20,8),
    status              VARCHAR(32) NOT NULL,
    review_reason       TEXT,
    failure_reason      TEXT,
    retry_count         INTEGER NOT NULL DEFAULT 0,
    next_retry_at       TIMESTAMPTZ,
    detected_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    finalized_at        TIMESTAMPTZ,
    credited_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT web3_deposits_event_uniq UNIQUE (chain_id, tx_hash, log_index)
);
```

必要索引：

```text
(status, next_retry_at, id)
(user_id, created_at DESC, id DESC)
(block_number, id)
(deposit_address_id, created_at DESC)
lower(tx_hash)
```

相同交易中向同一地址发送两次 Token 时，两个不同 `log_index` 必须生成两条充值记录并分别入账。

### 8. 状态机显式区分链状态与业务处置

状态：

```text
detected
  -> confirming
  -> ready_to_credit
  -> crediting
  -> credited

detected|confirming
  -> below_minimum
  -> manual_review
  -> orphaned

ready_to_credit|crediting
  -> failed

manual_review
  -> ready_to_credit        (管理员批准且重新验证通过)
  -> ignored                (管理员明确不入账并填写原因)

failed
  -> ready_to_credit        (自动或人工重试)
```

规则：

- `< 1 USDT0`：进入 `below_minimum`，MVP 不累计，不自动入账。
- `1 <= amount <= 10000 USDT0`：finalized 后进入 `ready_to_credit`。
- `amount > 10000 USDT0`：finalized 后进入 `manual_review`。
- 用户已软删除、地址记录异常或金额超出平台余额范围：进入 `manual_review`。
- `credited` 后不允许自动改为 `orphaned`；finalized 后极端重组属于安全事件，由人工冻结/调账流程处理。

### 9. 扫描分发现和最终确认两阶段

#### 发现循环

发现循环读取 near-head 区块的 USDT0 `Transfer` 日志，使用户尽快看到“确认中”：

1. 获取 latest block。
2. 从 `last_scanned_block - overlap_blocks` 开始分段调用 `eth_getLogs`。
3. filter address 固定为 USDT0 Token，topic0 固定为 Transfer。
4. 解析 `to`，批量匹配 `web3_deposit_addresses.normalized_address`。
5. 对匹配日志执行幂等 upsert；不匹配地址不落业务表。
6. 保存每条日志的 block hash，并推进发现游标。

#### 最终确认循环

1. 调用 `eth_getBlockByNumber("finalized", false)` 获取 finalized block。
2. 查询 `detected/confirming` 且 `block_number <= finalized.number` 的充值。
3. 重新读取 canonical block 和 transaction receipt。
4. 验证 receipt success、receipt block hash、Token 地址、topic、log index、to 和 raw amount 全部一致。
5. 不一致或日志消失则标记 `orphaned`。
6. 一致则按金额和用户状态进入 `ready_to_credit`、`manual_review` 或 `below_minimum`。

发现循环和最终确认循环可属于同一 runtime，但必须使用不同游标/指标，不能因为 finalizer 暂停而阻止继续发现。

### 10. 游标必须可恢复并支持多实例

建议结构：

```sql
CREATE TABLE web3_scanner_cursors (
    scanner_key          VARCHAR(128) PRIMARY KEY,
    chain_id             BIGINT NOT NULL,
    token_contract       VARCHAR(42) NOT NULL,
    scan_start_block     BIGINT NOT NULL,
    last_scanned_block   BIGINT NOT NULL,
    last_finalized_block BIGINT NOT NULL,
    last_error           TEXT,
    last_success_at      TIMESTAMPTZ,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

运行时要求：

- 使用现有领导锁能力，只有 leader 扫描和推进游标。
- 失去租约后当前批次不得继续写入游标；幂等插入允许重复发现。
- `scan_start_block` 初始化后不可向后修改；需要历史补扫时使用独立管理命令和明确区间。
- 每批区块范围可配置；RPC 返回范围过大错误时递减 batch size。
- 默认重叠重扫一段最近区块，具体值由配置和生产演练决定。

### 11. RPC 配置是运维配置，不是普通管理员输入

配置使用“钱包映射 + 网络映射 + 资产映射”的层级结构。Conflux eSpace 和 USDT0 是首期唯一启用目标，但未来新增 EVM 兼容链或 Token 时只增加映射条目，不修改配置 schema：

```yaml
web3_deposit:
  enabled: false
  scanner_enabled: false
  credit_enabled: false
  user_entry_enabled: false
  wallets:
    evm_deposit_v1:
      account_xpub: "..."
      account_path: "m/44'/60'/0'"
  networks:
    conflux_espace_mainnet:
      enabled: false
      chain_id: 1030
      wallet_id: evm_deposit_v1
      scan_start_block: 0
      rpc_urls: []
      poll_interval_seconds: 15
      block_batch_size: 500
      overlap_blocks: 20
      assets:
        usdt0:
          contract_address: "0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff"
          decimals: 6
          minimum_deposit: "1.000000"
          auto_credit_limit: "10000.000000"
```

环境变量沿用相同层级，例如：

```text
WEB3_DEPOSIT_ENABLED=false
WEB3_DEPOSIT_WALLETS_EVM_DEPOSIT_V1_ACCOUNT_XPUB=...
WEB3_DEPOSIT_NETWORKS_CONFLUX_ESPACE_MAINNET_SCAN_START_BLOCK=...
WEB3_DEPOSIT_NETWORKS_CONFLUX_ESPACE_MAINNET_RPC_URLS=https://rpc-1.example,https://rpc-2.example
```

启动预检失败时：

- 若功能关闭，记录不含敏感值的 disabled 状态。
- 若功能开启，chain ID、Token code、decimals 或 finalized tag 任一不匹配，runtime 必须保持 unhealthy 且不得分配新地址或入账。
- 多 RPC 节点必须分别预检；错误节点隔离，至少一个健康节点才能运行。

禁止把任意管理员 URL 直接交给 RPC Client，以减少 SSRF 和错误链风险。

### 12. Web3 资金使用三表子账户模型

- `web3_deposits`：不可变的链上充值事实；`credited` 表示金额已经进入 Web3 子账户。
- `web3_user_balances`：按 `user_id + asset_key` 聚合的当前可用余额快照，并保存累计充值、累计划转和乐观锁版本。
- `web3_balance_transfers`：从 Web3 子账户划转到 `users.balance` 的不可变完成事实，保存划转前后双方余额和唯一幂等键。

`web3_balance_transfers.web3_balance_id + user_id` 必须通过复合外键引用同一条 `web3_user_balances`，数据库不得允许把一个用户的划转挂到另一个用户的 Web3 余额。

必须满足以下对账关系：

```text
SUM(credited web3_deposits.credited_amount)
- SUM(web3_balance_transfers.amount)
= web3_user_balances.available_amount

web3_user_balances.total_deposited
- web3_user_balances.total_transferred
= web3_user_balances.available_amount
```

初始资产键为 `usdt`。各受支持链上的 USDT 或 USDT0 充值都映射到这个平台内部资产键，从而汇总到同一余额；独立余额表仍允许未来增加其他资产，而不向 `users` 表持续增加列。

### 13. 充值入账与主余额划转分别保持事务原子性

`CreditDeposit` 在单个 PostgreSQL 事务内：

1. 锁定 `web3_deposits`，只接受可入账状态。
2. 锁定或创建对应 `web3_user_balances`。
3. 使用 SQL decimal 表达式增加 `available_amount` 与 `total_deposited`，不经 `float64`。
4. 将充值标记为 `credited` 并写入 `credited_amount/credited_at`。
5. 提交事务。

`TransferToMainBalance` 在另一个 PostgreSQL 事务内：

1. 使用唯一 `idempotency_key` 检查是否已经完成。
2. 锁定 `web3_user_balances` 并校验可用金额。
3. 锁定 `users`，使用 SQL decimal 表达式增加 `users.balance` 和 `total_recharged`。
4. 减少 Web3 可用余额、增加 `total_transferred` 和 `balance_version`。
5. 插入包含双方余额快照的 `web3_balance_transfers`。
6. 提交事务。

事务提交后再执行：

- 异步失效 billing/user balance cache。
- 发送充值成功通知。
- 写结构化日志和指标。

通知失败不得回滚余额，也不得导致重复划转；使用 transfer idempotency key 或独立通知幂等键重试。

### 14. 不复用充值码，不创建支付订单

Web3 充值不创建 `PaymentOrder`，也不生成 `RedeemCode`。原因：

- 用户没有预创建订单。
- 充值金额来自链上日志而不是订单期望金额。
- 充值码幂等键不能表达 `log_index`。
- 订单退款和过期状态不适用于不可逆链上转账。

可以抽取以下共享后处理接口：

```go
type BalanceCreditPostProcessor interface {
    InvalidateBalanceCache(ctx context.Context, userID int64)
    NotifyRechargeSuccess(ctx context.Context, event RechargeSuccessEvent) error
}
```

普通支付现有行为保持不变。

### 15. 用户 API 使用现有支付鉴权边界

建议挂载到 `/api/v1/payment/web3`：

| Method | Path | 行为 |
| --- | --- | --- |
| GET | `/config` | 返回固定网络、Token、最小金额、自动入账上限、最终性说明和功能状态 |
| POST | `/address` | 幂等创建或返回当前用户充值地址 |
| GET | `/address` | 获取已分配地址；未分配返回明确空状态，不产生写操作 |
| GET | `/deposits` | 游标或页码分页返回当前用户充值记录 |
| GET | `/deposits/:id` | 返回当前用户单条充值详情和确认状态 |

用户 API 不返回：

- derivation index；
- wallet xpub/fingerprint；
- scanner cursor；
- 内部失败堆栈；
- 其他用户地址或交易。

二维码 MVP 只编码地址文本。页面必须同时展示 `Conflux eSpace`、USDT0 合约和“不要使用 Core Space/其他 Token”的警告。

### 16. 管理 API 只允许安全处置

建议挂载到 `/api/v1/admin/payment/web3`：

| Method | Path | 行为 |
| --- | --- | --- |
| GET | `/deposits` | 按状态、用户、地址、tx hash、时间筛选 |
| GET | `/deposits/:id` | 查看链上事实、验证状态和 Web3 子账户入账结果 |
| POST | `/deposits/:id/approve` | 对 `manual_review` 重新链上验证后批准入账 |
| POST | `/deposits/:id/retry` | 重试 `failed` 入账或验证 |
| POST | `/deposits/:id/ignore` | 明确不入账，必须提供原因和二次确认 |
| GET | `/runtime` | RPC、leader、游标、latest/finalized 高度和延迟 |
| POST | `/runtime/rescan` | 创建受限区间补扫任务，不直接任意改游标 |

所有管理写操作复用现有管理员鉴权、step-up 要求和管理操作审计。批准操作不能修改链上金额、用户、Token 或收款地址。

### 17. 用户展示状态与内部状态分离

用户只看到：

```text
confirming
credited
below_minimum
under_review
failed
```

`detected/confirming/ready_to_credit/crediting` 可映射为不同阶段的 `confirming`，避免暴露内部租约和 Worker 状态。详情可展示交易哈希、金额、检测时间、最终确认时间和到账时间。

### 18. 删除或禁用用户的充值进入人工审核

地址必须继续监听，但自动入账前检查用户状态：

- 活跃用户：按金额规则处理。
- 禁用、封禁或软删除用户：进入 `manual_review`，不得静默丢弃。
- 用户记录不存在属于数据完整性安全事件，停止自动入账并告警。

### 19. 可观测性使用稳定字段

结构化日志至少包含：

```text
component=web3_deposit
chain_id
token_contract
scanner_key
deposit_id
tx_hash
log_index
block_number
status_before
status_after
rpc_endpoint_id
leader
retry_count
```

不得记录完整 xpub、助记词、私钥、Authorization header 或 RPC URL 中的凭据。

指标至少包含：

- latest/finalized/last scanned block 与延迟；
- RPC 请求耗时、错误和 failover；
- 各充值状态数量；
- detected 到 finalized、finalized 到 credited 延迟；
- 重复日志、孤块、人工审核、失败重试和余额入账失败数量；
- 地址分配总量和分配失败数量。

## Risks / Trade-offs

- **资金分散**：首期不归集，资金停留在用户 EOA；第二期需要为每个地址补充 CFX 并逐笔归集。
- **xpub 隐私**：xpub 不能花费资金，但泄露会暴露整棵地址树和资金活动，应按敏感配置保护。
- **余额历史不统一**：MVP 只为 Web3 子账户和向主余额划转建立专用事实；现有支付仍通过充值码和支付审计追踪。
- **finalized 延迟**：用户到账慢于 safe/near-head，但实现与风险语义更简单。
- **小额资金滞留**：低于 1 USDT0 的充值不累计，资金仍在地址中；页面必须明确提示。
- **RPC 供应商风险**：错误或滞后的 finalized 结果可能影响确认；启动预检、多个端点和链上重验证降低风险。
- **深度重组**：finalized 后仍出现异常属于链级安全事件，MVP 不自动扣回已消费余额。
- **未来归集兼容**：`wallet_id + derivation_index` 必须从第一天准确持久化，否则第二期无法可靠恢复私钥路径。

## Migration Plan

### 阶段 0：密钥和官方参数准备

1. 离线生成专用助记词和账户级 xpub。
2. 双人完成种子备份、恢复和已知地址重派生。
3. 固定 `wallet_id` 和账户路径。
4. 核对官方 Token 合约、decimals 和 RPC finalized 支持。

### 阶段 1：数据库和纯函数

1. 应用 migration 和 Ent schema。
2. 实现金额、地址规范化、Transfer 解析和 HD 公钥派生。
3. 实现地址并发 get-or-create。

### 阶段 2：只观察扫描器

1. scanner 启动但 `crediting_enabled=false`。
2. 使用内部测试地址和真实小额充值验证发现、finalized 和重扫。
3. 对照浏览器/RPC 确认日志、金额和区块哈希。

### 阶段 3：入账和管理闭环

1. 启用 Web3 子账户入账和向主余额的事务性划转。
2. 上线管理员查询、批准和重试。
3. 验证缓存、通知和 `total_recharged`。

### 阶段 4：用户灰度

1. 仅管理员/灰度用户显示充值入口。
2. 完成真实小额到账演练和故障演练。
3. 观察 finalized 延迟、RPC 错误和重复扫描至少一个完整运行窗口。
4. 全量开放。

### 回滚

- 关闭 `WEB3_DEPOSIT_ENABLED` 停止新地址分配和扫描器写入。
- 已分配地址和已发现充值记录不得删除。
- 已 credited 余额不得通过 migration 回滚；需要业务调账时走审计流程。
- 数据库表和列保留，避免用户继续向旧地址充值后失去映射。

## Resolved Decisions

- 使用 Conflux eSpace Mainnet 官方 USDT0 Token，不使用 OFT 合约作为充值地址。
- 平台余额是 USD，USDT0 固定 1:1 入账。
- 使用 HD 派生 EOA，主服务只持有 xpub。
- 首期不自动归集。
- 最低充值 1 USDT0，手续费 0。
- 大于 10,000 USDT0 人工审核。
- finalized 后才入账。
- MVP 不发放邀请返佣。
