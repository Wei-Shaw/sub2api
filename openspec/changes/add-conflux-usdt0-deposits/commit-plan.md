# Conflux USDT0 充值小步提交计划

本文把 `tasks.md` 中的实施任务拆分为可独立审查、独立验证和安全回滚的小提交。实现时按顺序推进；除明确标记为文档或依赖的提交外，每个提交都必须同时包含直接对应的测试。

## 1. 提交纪律

1. 每个提交只引入一个可描述的能力，不混入无关重构、格式化或依赖升级。
2. 实现与直接测试放在同一个提交；不要先提交不可验证的实现，再用后续提交补测试。
3. Ent schema、生成代码、SQL migration 和该表的 migration 测试放在同一个提交。
4. 每个提交结束时必须能够编译；后端至少运行受影响包测试，阶段结束运行全量测试。
5. 链上金额不得经过 `float64`，主服务不得持有任何可签名私钥。
6. 功能默认关闭，并分别控制总开关、Scanner、Credit Worker 和用户入口。
7. migration 编号在创建文件前根据目标分支重新确认，本文中的 `196` 起始编号仅为当前计划占位。
8. Scanner、Finalizer 和 Credit Worker 不得在同一提交中同时接入运行时。

建议从目标集成分支创建独立功能分支：

```bash
git switch -c feat/conflux-usdt0-deposits
```

## 2. 阶段一：规格与基础结构

### Commit 01

```text
docs: add conflux usdt0 deposit specification
```

- 提交本 OpenSpec 目录中的 proposal、design、tasks、verification、source baseline 和实施文档。
- 不包含产品代码。

### Commit 02

```text
chore(web3deposit): add evm rpc dependencies
```

- 引入最小必要的 go-ethereum RPC、地址、Hash 和 Log 类型依赖。
- 只修改 `go.mod` 和 `go.sum`。

### Commit 03

```text
chore(web3deposit): add public hd derivation dependency
```

- 引入支持账户级 xpub 非 hardened 子公钥派生的库。
- 与 EVM/RPC 依赖分开审查。

### Commit 04

```text
feat(web3deposit): add disabled deposit configuration
```

- 增加网络、Token、RPC、xpub、扫描区间和金额规则配置。
- 增加 `enabled`、`scanner_enabled`、`credit_enabled`、`user_entry_enabled`，全部默认关闭。
- 固定 Chain ID `1030`、USDT0 合约和 decimals `6`。

### Commit 05

```text
feat(web3deposit): validate deposit configuration
```

- 校验 RPC endpoints、账户级 xpub、wallet ID、扫描起点和金额边界。
- 配置不一致时 fail closed。
- 验证日志和错误信息不会输出完整 xpub。

### Commit 06

```text
feat(web3deposit): define deposit domain types
```

- 定义链配置、事件 ID、Transfer 事件、充值状态和允许的状态转换。
- 核心领域类型不得依赖 Ent entity。

### Commit 07

```text
feat(web3deposit): add exact usdt0 amount conversion
```

- 使用 `big.Int` 保存 ERC20 raw amount。
- 使用 `decimal.Decimal` 转换六位 Token 金额和平台入账金额。
- 覆盖零值、边界值、超大 uint256 和 `DECIMAL(20,8)` 溢出。

### Commit 08

```text
feat(web3deposit): serialize monetary values as strings
```

- 定义用户端和管理员端金额 DTO。
- JSON 金额只输出字符串，不输出 JSON number。

## 3. 阶段二：数据库基础

每个提交包含对应 migration、Ent schema、生成代码、migration 测试和最小仓储测试。

### Commit 09

```text
feat(web3deposit): persist hd wallet metadata
```

- 新增 `web3_deposit_wallets`。
- 保存 wallet ID、account path、xpub fingerprint 和下一个派生索引。
- 不保存完整 xpub。

### Commit 10

```text
feat(web3deposit): persist user deposit addresses
```

- 新增 `web3_deposit_addresses`。
- 添加用户、钱包、派生索引和 normalized address 唯一约束。
- 地址历史不随用户软删除而删除。

### Commit 11

```text
feat(web3deposit): persist detected deposit events
```

- 新增 `web3_deposits`。
- 唯一事件键为 `chain_id + tx_hash + log_index`。
- 添加状态、Worker claim 和用户历史查询索引。

### Commit 12

```text
feat(web3deposit): persist scanner cursors and leases
```

- 新增 Scanner、Finalizer 游标和运行租约。
- 游标初始化和正常推进只能单调向前。

### Commit 13

```text
feat(web3deposit): add balance ledger entries
```

- 新增 `balance_ledger_entries`。
- 添加唯一 `idempotency_key`。
- 此提交不修改用户余额。

## 4. 阶段三：HD 地址分配

### Commit 14

```text
feat(web3deposit): derive evm addresses from account xpub
```

- 实现账户级 xpub 的 `/0/{index}` 非 hardened 派生。
- 输出 checksum 地址和 lowercase normalized address。
- 使用固定 xpub/index/address 测试向量锁定结果。

### Commit 15

```text
feat(web3deposit): verify wallet fingerprint at startup
```

- 计算运行配置 xpub fingerprint。
- 与数据库钱包 metadata 比较。
- 不匹配时禁止地址分配和 Scanner 启动。

### Commit 16

```text
feat(web3deposit): reserve derivation indexes atomically
```

- 在事务中锁定钱包行并原子增加 `next_derivation_index`。
- 派生失败时允许索引空洞，不回退计数器。

### Commit 17

```text
feat(web3deposit): get or create user deposit address
```

- 实现用户长期地址懒分配。
- 唯一冲突后读取已有地址。
- 覆盖 100 路并发请求只产生一个用户地址。

### Commit 18

```text
feat(web3deposit): expose user deposit configuration
```

- 增加 `/api/v1/payment/web3/config`。
- 只返回公开网络、Token、最低金额和功能状态。
- 不返回 xpub、RPC 凭据或扫描内部配置。

### Commit 19

```text
feat(web3deposit): expose user deposit address
```

- 增加 `/api/v1/payment/web3/address`。
- 接入 JWT、用户模式 Guard 和限流。
- 功能关闭时不得创建地址。

阶段门禁：用户能够获取长期充值地址，但系统尚未扫描链上事件，也不会修改余额。

## 5. 阶段四：Conflux RPC 与日志解析

### Commit 20

```text
feat(web3deposit): add conflux rpc endpoint pool
```

- 实现请求 deadline、endpoint 健康状态和 failover。
- 暂时只暴露扫描所需的基础 JSON-RPC 能力。

### Commit 21

```text
feat(web3deposit): verify conflux network at startup
```

- 验证 `eth_chainId == 1030`。
- 验证 Token 合约存在 bytecode、decimals 为 `6`。
- 验证 endpoint 支持 `finalized` tag。

### Commit 22

```text
feat(web3deposit): parse erc20 transfer logs
```

- 严格解析 `Transfer(address,address,uint256)`。
- 验证 topic 数量、data 长度、地址和 raw amount。
- 拒绝 malformed、removed 和非目标合约日志。

### Commit 23

```text
feat(web3deposit): fetch transfer logs in bounded ranges
```

- 实现固定 USDT0 合约范围的 `eth_getLogs`。
- 对区间过大错误收缩 batch size。
- 所有重试必须有明确上限。

## 6. 阶段五：Scanner

### Commit 24

```text
feat(web3deposit): match transfer recipients to deposit addresses
```

- 批量查询 normalized address。
- 只返回平台已分配的地址。
- 不匹配日志不创建业务记录。

### Commit 25

```text
feat(web3deposit): upsert detected deposit events
```

- 根据事件唯一键幂等插入充值事实。
- 同一交易的不同 `log_index` 创建独立记录。
- 重复日志不得改变已保存的原始链上事实。

### Commit 26

```text
feat(web3deposit): advance scanner cursor atomically
```

- 完整批次成功后才推进游标。
- 任意日志写入失败时游标保持不变。

### Commit 27

```text
feat(web3deposit): scan with overlap and deterministic ordering
```

- 从 `last_scanned_block - overlap_blocks` 开始重扫。
- 对 RPC 返回日志稳定排序。
- 覆盖重复批次和进程重启。

### Commit 28

```text
feat(web3deposit): run scanner under database lease
```

- 多实例只有租约持有者运行 Scanner。
- 实现续租、租约丢失和优雅停止。
- 此阶段必须保持 `credit_enabled=false`。

阶段门禁：系统能够观察真实链上充值并生成确认中记录，但不会增加用户余额。

## 7. 阶段六：Finalizer

### Commit 29

```text
feat(web3deposit): verify canonical deposit receipts
```

- 重新读取 canonical block 和 transaction receipt。
- 验证 receipt status、block hash、Token、log index、to 和 amount。

### Commit 30

```text
feat(web3deposit): classify finalized deposit amounts
```

- `< 1 USDT0` 进入 `below_minimum`。
- `1 <= amount <= 10000 USDT0` 进入 `ready_to_credit`。
- `> 10000 USDT0` 进入 `manual_review`。

### Commit 31

```text
feat(web3deposit): route inactive users to manual review
```

- 用户软删除、禁用或地址关系异常时进入人工审核。
- 不允许这些记录自动入账。

### Commit 32

```text
feat(web3deposit): finalize deposits under independent cursor
```

- 读取 RPC `finalized` block。
- 批量重新验证待确认充值。
- 独立维护 Finalizer cursor。
- 日志消失或验证不一致时标记 `orphaned`。

## 8. 阶段七：事务性余额入账

### Commit 33

```text
feat(web3deposit): credit deposits in one transaction
```

同一事务内完成：

1. 锁定充值记录。
2. 锁定用户记录。
3. 创建唯一 ledger entry。
4. 精确增加 `balance` 和 `total_recharged`。
5. 将充值标记为 `credited`。

不得调用基于 `float64` 的余额调整接口。

### Commit 34

```text
feat(web3deposit): claim credit jobs with recovery lease
```

- 条件 claim `ready_to_credit` 记录。
- 支持多个 Credit Worker。
- Worker 崩溃后租约到期可恢复。

### Commit 35

```text
feat(web3deposit): invalidate caches after deposit commit
```

- 只在数据库事务提交成功后失效余额和认证缓存。
- 缓存失效失败不得回滚已完成的入账。

### Commit 36

```text
feat(web3deposit): notify users after successful credit
```

- 通过窄接口复用充值成功通知。
- 通知失败可以重试，但不得重复增加余额。

### Commit 37

```text
test(web3deposit): harden credit idempotency
```

- 100 路重复履约只能入账一次。
- ledger 唯一冲突不得重复增加余额。
- 覆盖事务各阶段故障注入。
- 证明 Web3 充值不会创建 PaymentOrder、RedeemCode 或邀请返佣。

阶段门禁：只对管理员灰度用户开启 `credit_enabled`，完成真实小额充值和数据库对账。

## 9. 阶段八：用户查询与页面

### Commit 38

```text
feat(web3deposit): expose user deposit history
```

- 增加用户充值列表和详情 API。
- 使用稳定分页。
- 用户只能访问自己的充值记录。

### Commit 39

```text
feat(web3deposit): add frontend deposit api contracts
```

- 增加 TypeScript 类型和 API client。
- 金额字段保持字符串类型。

### Commit 40

```text
feat(web3deposit): add user deposit address view
```

- 展示网络、Token、合约、充值地址和二维码。
- 增加复制能力和误充值风险提示。

### Commit 41

```text
feat(web3deposit): add user deposit history view
```

- 展示 confirming、credited、below minimum、manual review 和 failed 状态。
- 不包含管理员操作。

### Commit 42

```text
feat(web3deposit): add deposit navigation and translations
```

- 增加用户导航入口。
- 增加中英文 i18n。
- 入口受 `user_entry_enabled` 控制。

## 10. 阶段九：管理员能力

### Commit 43

```text
feat(web3deposit): expose admin deposit queries
```

- 增加筛选、详情、状态统计和链高度延迟查询。
- 默认突出 `manual_review` 和 `failed`。

### Commit 44

```text
feat(web3deposit): approve or ignore reviewed deposits
```

- approve 前重新验证链上事实。
- ignore 必须填写理由。
- 管理员不能修改金额、用户、Token、交易 Hash 或 finalized 事实。

### Commit 45

```text
feat(web3deposit): retry failed deposit credits
```

- 只允许合法状态进入重试。
- 重试继续依赖 ledger 和事件唯一键保证幂等。

### Commit 46

```text
feat(web3deposit): add bounded rescan operations
```

- 只允许指定起止区块的补扫。
- 不允许管理员直接修改生产游标。

### Commit 47

```text
feat(web3deposit): audit admin deposit operations
```

- approve、ignore、retry 和 rescan 写入管理员操作审计。
- 按项目现有能力接入 step-up 验证。

### Commit 48

```text
feat(web3deposit): add admin deposit console
```

- 增加管理员列表、详情、审核、重试和运行状态页面。
- 前端不得编辑链上事实字段。

## 11. 阶段十：可观测性和上线

### Commit 49

```text
feat(web3deposit): add runtime metrics and structured logs
```

- 增加 RPC 健康、扫描延迟和 Finalizer 延迟指标。
- 增加状态堆积、孤块、重试和入账失败指标。
- 所有结构化日志不得包含完整 xpub。

### Commit 50

```text
docs(web3deposit): add operations and rollout runbook
```

- 记录 RPC 切换、Scanner 停止、补扫和人工审核流程。
- 记录钱包恢复和 fingerprint 验证流程。
- 明确上线顺序：migration、只观察 Scanner、真实小额验证、管理员灰度入账、用户入口。

## 12. 阶段验证门禁

| 阶段 | 必须通过 |
| --- | --- |
| 配置与领域 | 配置默认关闭、敏感信息脱敏、金额精度、溢出和状态转换测试 |
| 数据库 | 空库迁移、升级迁移、唯一约束、CHECK 约束和典型查询验证 |
| 地址分配 | 固定派生向量、100 路并发和 fingerprint fail-closed |
| RPC 与 Scanner | 错误链、错误 Token、RPC failover、重复扫描、批次失败和领导权切换 |
| Finalizer | canonical receipt、重组、日志消失、金额分类和用户异常状态 |
| 余额入账 | 100 路并发幂等、事务故障注入、缓存失败和通知失败 |
| API | 鉴权、越权、金额字符串契约和功能关闭行为 |
| 前端 | TypeScript 类型检查、定向 Vitest 和生产构建 |
| 上线 | 真实小额充值对账、重启演练、RPC 故障演练和大额审核演练 |

## 13. 建议验证命令

每个后端提交先运行受影响包：

```bash
cd backend
go test ./internal/web3deposit/... -count=1
```

涉及配置、仓储、Handler 或路由时增加对应包测试。每个阶段结束运行：

```bash
cd backend
go test ./... -count=1
```

前端提交运行：

```bash
cd frontend
npm run typecheck
npm run test:run
npm run build
```

## 14. 回滚策略

1. 优先关闭 `user_entry_enabled`，阻止新增用户入口和地址分配。
2. 关闭 `credit_enabled`，停止余额入账但保留链上观察和记录。
3. 关闭 `scanner_enabled`，停止链上读取，保留已经发现的事实和游标。
4. 最后关闭总 `enabled` 开关。
5. 已创建的地址、充值事实、ledger 和游标不得通过功能回滚删除。
6. 已应用的 migration 不修改、不删除；通过新的向前 migration 修复数据库问题。
