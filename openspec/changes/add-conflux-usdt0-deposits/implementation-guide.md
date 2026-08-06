# 实施指导

## 1. 不可变边界

实施过程中以下约束优先于文件复用便利：

1. 链上金额不得经过 `float64`。
2. 主服务不得拥有任何可签名私钥。
3. 充值事实不得存入 `payment_orders` 或 `redeem_codes`。
4. 余额增加必须与 ledger、充值状态处于同一数据库事务。
5. 只有 `finalized` 且重新验证通过的日志可自动入账。
6. 地址历史映射不得因用户停用、钱包轮换或功能关闭而删除。
7. 所有重试都必须依赖数据库幂等约束，而不是进程内标记。

## 2. 依赖方向

```text
handler/admin_handler
        ↓
address_service / query_service / review_service
        ↓
domain ports
        ↓
postgres_repository / rpc_client / hd_deriver

runtime
  -> scanner
  -> finalizer
  -> credit_worker

credit_service
  -> exact balance transaction port
  -> post-commit cache/notification port
```

`web3deposit` 可以依赖稳定的 service/repository 接口，但现有 `PaymentService` 不应成为其父对象。共享充值成功通知通过窄接口注入。

## 3. 建议新增文件

### 后端

```text
backend/ent/schema/web3_deposit_wallet.go
backend/ent/schema/web3_deposit_address.go
backend/ent/schema/web3_deposit.go
backend/ent/schema/web3_scanner_cursor.go
backend/ent/schema/balance_ledger_entry.go

backend/internal/web3deposit/config.go
backend/internal/web3deposit/domain.go
backend/internal/web3deposit/amount.go
backend/internal/web3deposit/address_deriver.go
backend/internal/web3deposit/address_service.go
backend/internal/web3deposit/log_parser.go
backend/internal/web3deposit/rpc_client.go
backend/internal/web3deposit/scanner.go
backend/internal/web3deposit/finalizer.go
backend/internal/web3deposit/credit_service.go
backend/internal/web3deposit/query_service.go
backend/internal/web3deposit/review_service.go
backend/internal/web3deposit/postgres_repository.go
backend/internal/web3deposit/handler.go
backend/internal/web3deposit/admin_handler.go
backend/internal/web3deposit/runtime.go
backend/internal/web3deposit/metrics.go
backend/internal/web3deposit/wire.go

backend/internal/server/routes/web3_deposit.go
```

### 前端

```text
frontend/src/api/web3Deposit.ts
frontend/src/types/web3Deposit.ts
frontend/src/views/user/Web3DepositView.vue
frontend/src/views/admin/Web3DepositsView.vue
frontend/src/components/web3-deposit/DepositAddressCard.vue
frontend/src/components/web3-deposit/DepositHistoryTable.vue
frontend/src/components/web3-deposit/DepositStatusBadge.vue
```

具体路径可以按现有 feature 组织调整，但用户和管理员类型、API 与视图应保持边界清晰。

## 4. 推荐依赖

后端需要成熟的 EVM primitives：

- `github.com/ethereum/go-ethereum/rpc`：调用 `eth_chainId`、`eth_getLogs`、`eth_getBlockByNumber` 和 receipt RPC。
- `github.com/ethereum/go-ethereum/common`、`core/types`、`crypto`：地址、哈希、日志和 Keccak。
- 维护中的 BIP32/xpub 库：从账户级 xpub 派生 `/0/index` 非 hardened 子公钥。
- 已存在的 `github.com/shopspring/decimal`：Token 和 USD 金额。

不要为了 MVP 引入完整钱包守护进程、消息队列或区块索引服务。若 BIP32 库选择发生变化，必须以固定 xpub/index/address 测试向量锁定行为。

## 5. 公共领域类型

建议避免在核心类型中暴露 Ent entity：

```go
type ChainConfig struct {
    ChainID          uint64
    TokenAddress     common.Address
    TokenDecimals    int32
    WalletID         string
    AccountPath      string
    ScanStartBlock   uint64
    MinimumDeposit   decimal.Decimal
    AutoCreditLimit  decimal.Decimal
}

type DepositEventID struct {
    ChainID uint64
    TxHash  common.Hash
    LogIndex uint
}

type TransferEvent struct {
    ID          DepositEventID
    BlockNumber uint64
    BlockHash   common.Hash
    From        common.Address
    To          common.Address
    RawAmount   *big.Int
}
```

`*big.Int` 进入对象后不得由调用方继续修改；构造函数应复制数值。

## 6. HD 地址派生

### 6.1 配置加载

配置加载时：

1. 解析账户级 xpub。
2. 拒绝私钥扩展 key。
3. 验证只能派生非 hardened `/0/index`。
4. 计算稳定 fingerprint，与 `web3_deposit_wallets` 中记录比较。
5. 派生固定健康检查 index，并验证输出为合法 20-byte EVM 地址。

### 6.2 并发分配

首选数据库事务：

```sql
SELECT * FROM web3_deposit_wallets
WHERE wallet_id = $1
FOR UPDATE;

UPDATE web3_deposit_wallets
SET next_derivation_index = next_derivation_index + 1
WHERE wallet_id = $1
RETURNING next_derivation_index - 1;
```

派生失败时允许 index 空洞，不得回退计数器。插入地址失败时根据唯一约束判断：

- 用户唯一冲突：读取并返回已有地址。
- index/address 冲突：告警并停止分配，不盲目重试大量 index。

## 7. RPC Client

### 7.1 启动预检

对每个 endpoint 执行：

```text
eth_chainId == 0x406
eth_getCode(token, latest) != 0x
eth_call decimals() == 6
eth_getBlockByNumber(finalized, false) returns valid block
```

给 endpoint 使用内部 ID，日志只输出 ID 和主机脱敏值，不输出 URL query/userinfo。

### 7.2 finalized block

`ethclient` 的 block number API 主要接收整数；读取 `finalized` 时使用底层 `rpc.Client.CallContext`，明确传入字符串 tag。不要自行用固定确认数模拟 finalized。

### 7.3 日志批次

RPC 查询固定：

```text
address = configured USDT0 token
topics[0] = Transfer(address,address,uint256)
fromBlock/toBlock = bounded range
```

不把所有用户地址拼成超大 topic OR filter。扫描 Token 的 Transfer 后，在本地缓存或数据库批量匹配 `to` 地址。

地址匹配缓存只是一种优化；缓存 miss 时仍需查询数据库，数据库唯一映射才是事实源。

## 8. Scanner 写入顺序

每个区块批次：

1. 拉取并排序日志：`blockNumber, transactionIndex, logIndex`。
2. 严格解析 topic 数量、地址 padding、data 长度和 uint256。
3. 去掉 removed 日志或按 orphan 候选处理。
4. 批量查询目标地址映射。
5. 幂等插入匹配充值。
6. 更新地址 `last_deposit_at`。
7. 只有整个批次成功后推进 cursor。

若批次中单条 malformed log，记录错误并使批次失败，不得跳过后继续推进游标造成永久漏扫。已插入记录由唯一键承受重试。

## 9. Finalizer 验证

对每个候选充值重新验证：

```text
canonical block hash at block_number == stored block_hash
receipt exists
receipt status == success
receipt blockHash == stored block_hash
receipt.logs[log_index or matching tuple] exists
log.address == configured token
topic0 == Transfer
decoded to == stored to_address
decoded raw amount == stored raw_amount
```

不要假设 receipt logs slice 下标等于全局 `log_index`；应按 RPC `logIndex` 字段或完整事件元组匹配。

验证成功后使用条件更新转换状态，避免旧 Worker 覆盖管理员操作。

## 10. Exact Balance Transaction

建议为 Web3 模块提供专用 repository 方法，而不是扩展现有 `AdjustBalance(float64)`：

```go
type CreditResult struct {
    BalanceBefore decimal.Decimal
    BalanceAfter  decimal.Decimal
    AlreadyDone   bool
}

type BalanceCreditor interface {
    CreditWeb3Deposit(ctx context.Context, depositID int64) (CreditResult, error)
}
```

实现可使用 `database/sql` + PostgreSQL `NUMERIC` 文本扫描，或为 `decimal.Decimal` 编写 Ent ValueScanner。关键是 transaction 内所有金额都保持 decimal。

推荐 SQL 使用行锁并返回精确值，不在 Go 中读取 float 再写回：

```sql
UPDATE users
SET balance = balance + $1::numeric,
    total_recharged = total_recharged + $1::numeric
WHERE id = $2
RETURNING balance, total_recharged;
```

为得到 `balance_before`，先在同一事务 `SELECT balance FOR UPDATE`，再更新并写 ledger。

## 11. Runtime 生命周期

`Runtime` 提供：

```go
Start(ctx context.Context) error
Stop()
Status() RuntimeStatus
```

Wire 创建后由 provider 启动，统一 cleanup 调用 `Stop`。内部 goroutine 必须：

- 继承可取消 context；
- poll 使用 timer/ticker 且 Stop 可立即唤醒；
- RPC 调用有 deadline；
- leader lease 丢失时停止推进游标；
- panic 被捕获、记录并使 runtime unhealthy，不能静默退出。

## 12. API DTO

金额统一作为 JSON 字符串返回：

```json
{
  "amount": "123.456789",
  "minimum_deposit": "1.000000",
  "auto_credit_limit": "10000.000000"
}
```

不要返回 JSON number，避免浏览器浮点转换。交易哈希、合约和地址使用 checksum 或规范字符串，比较时始终 lowercase。

## 13. 前端实现

用户页面：

- 首次进入先 GET address；为空时由明确操作或页面初始化 POST get-or-create。
- 同时展示网络 `Conflux eSpace` 和 Token 合约，不能只显示 `USDT0`。
- 复制地址、复制 Token 合约分别提供反馈。
- 二维码只包含地址，不自动构造可能被钱包误解的跨链 URI。
- 充值记录自动刷新，但页面离开后停止 polling。
- 状态和金额文案来自后端，不在前端自行计算 finality 或阈值。

管理员页面：

- 默认突出 `manual_review` 和 `failed`。
- approve/ignore 使用二次确认并要求填写审核备注。
- 不提供修改链上金额、用户 ID、Token 或 tx hash 的表单。

## 14. 实施顺序

1. 配置结构、领域枚举、金额和日志解析纯函数。
2. SQL migration、Ent schema 和 repository contract。
3. xpub 派生和地址 get-or-create。
4. RPC 预检、scanner 和 cursor。
5. finalizer/canonical verifier。
6. exact-decimal ledger 和 credit worker。
7. 用户 API 与页面。
8. 管理 API 与页面。
9. metrics、告警、灰度开关和运行手册。

## 15. Definition of Done

- 所有 `verification.md` 中 P0 门禁通过。
- 生产配置中没有助记词或私钥。
- 已完成从备份种子重新派生测试用户地址的演练。
- scanner 关闭重启后不漏扫、不重复入账。
- 相同链上日志并发提交 100 次仍只有一条 ledger 和一次余额增加。
- 用户能看到真实小额充值从 confirming 到 credited。
- 管理员可以批准大额充值并留下完整审计。
- 功能关闭后历史地址与充值记录仍可查询。
