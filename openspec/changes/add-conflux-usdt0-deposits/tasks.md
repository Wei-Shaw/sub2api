## 1. 固定配置和密钥边界

- [x] 1.1 增加 Web3 Deposit 环境配置和启动校验，默认关闭。
- [ ] 1.2 离线生成专用 HD wallet，记录 `wallet_id`、账户路径和 xpub fingerprint。
- [x] 1.3 增加配置脱敏，证明日志、runtime API 和错误响应不包含完整 xpub。
- [x] 1.4 记录生产启用时的 `scan_start_block`，禁止运行时任意回退。

## 2. 建立领域类型和精确金额

- [x] 2.1 定义地址、充值、状态、链配置和事件 ID 类型。
- [x] 2.2 使用 `big.Int` 解析 ERC20 raw amount，使用 `decimal.Decimal` 计算 6 位 Token 金额。
- [x] 2.3 金额 API 使用字符串 DTO，不输出 JSON number。
- [x] 2.4 增加 0、边界值、超大 uint256、六位小数和 `DECIMAL(20,8)` 溢出测试。

## 3. 创建数据库迁移和仓储

- [x] 3.1 新增 `web3_deposit_wallets` 和原子 derivation index 分配。
- [x] 3.2 新增 `web3_deposit_addresses`、唯一约束和历史地址查询索引。
- [x] 3.3 新增 `web3_deposits`、状态 CHECK、事件唯一键和 Worker claim 索引。
- [x] 3.4 新增 `web3_scanner_cursors` 和不可回退的初始化逻辑。
- [x] 3.5 新增 `web3_user_balances`、`web3_balance_transfers` 和唯一 `idempotency_key`。
- [ ] 3.6 增加 migration 空库、升级、约束和典型查询 `EXPLAIN` 测试。

## 4. 实现 HD 地址分配

- [x] 4.1 选定并接入 EVM/BIP32 依赖。
- [x] 4.2 实现账户级 xpub `/0/index` 派生和 checksum/lowercase 地址规范化。
- [x] 4.3 增加跨实现已知测试向量，锁定 xpub、index 和地址结果。
- [x] 4.4 实现用户地址 get-or-create 事务和唯一冲突恢复。
- [x] 4.5 覆盖同一用户 100 路并发请求只分配一个地址。
- [x] 4.6 覆盖钱包 fingerprint 不匹配时 fail closed，禁止继续分配。

## 5. 实现 Conflux RPC 和日志解析

- [x] 5.1 实现 RPC endpoint 池、deadline、健康状态和 failover。
- [x] 5.2 启动时校验 chain ID `1030`、Token bytecode、decimals `6` 和 finalized tag。
- [x] 5.3 实现严格 ERC20 Transfer topic/data 解析。
- [x] 5.4 实现固定 Token 合约范围的分段 `eth_getLogs`。
- [x] 5.5 对 RPC range 错误实现 batch size 收缩和有界重试。
- [x] 5.6 增加错误链、错误 Token、malformed log、removed log 和 RPC 分叉测试。

## 6. 实现 Scanner 和游标

- [x] 6.1 实现 leader-only scanner runtime 和优雅启停。
- [x] 6.2 实现 overlap 重扫、日志排序、地址批量匹配和幂等 upsert。
- [x] 6.3 只有完整批次成功后推进 cursor。
- [x] 6.4 实现 scanner restart、leader 切换和重复批次恢复测试。
- [x] 6.5 实现持久化、可审计、可恢复的受限区间补扫任务，禁止直接任意修改生产游标。

## 7. 实现 Finalizer 和状态机

- [x] 7.1 读取 RPC `finalized` block 并推进独立 finalized cursor。
- [x] 7.2 重新验证 canonical block、receipt status、block hash 和目标 Transfer 日志。
- [x] 7.3 实现金额规则：低于 1、自动入账区间、大于 10,000 人工审核。
- [x] 7.4 实现禁用/软删除用户进入人工审核。
- [x] 7.5 实现 orphaned、manual_review、below_minimum 和 failed 转换测试。

## 8. 实现事务性余额入账

- [x] 8.1 新增 exact-decimal `CreditWeb3Deposit` 仓储事务，把 finalized 充值计入 `web3_user_balances`。
- [x] 8.2 新增 Web3 余额向 `users.balance` 的原子划转事务并写入 `web3_balance_transfers`。
- [x] 8.3 实现 credit lease 或条件 claim，支持多 Worker 和崩溃恢复。
- [x] 8.4 提交后失效余额缓存并发送充值成功通知。
- [x] 8.5 覆盖 100 路重复履约、划转幂等键冲突、事务中断和通知失败测试。
- [x] 8.6 确认 Web3 充值不创建 PaymentOrder、RedeemCode 或 affiliate rebate。

## 9. 实现用户 API 和页面

- [x] 9.1 注册 `/api/v1/payment/web3/config`、`address`、`deposits` 和详情路由。
- [x] 9.2 复用 JWT、BackendModeUserGuard 和用户面板限流。
- [x] 9.3 增加用户充值页面、地址二维码、复制能力、风险提示和历史记录。
- [x] 9.4 增加 loading、empty、disabled、confirming、credited、under review 和 error 状态。
- [x] 9.5 增加中文、英文和现有项目要求的 i18n 文案。
- [x] 9.6 增加 Web3 子余额读取、展示和幂等划转到主余额的用户入口。

## 10. 实现管理 API 和页面

- [x] 10.1 增加充值筛选、详情、runtime 健康和游标延迟 API。
- [x] 10.2 增加 approve、retry、ignore、bounded rescan 及补扫任务列表/详情。
- [x] 10.3 管理写操作接入管理员鉴权、step-up 和操作审计。
- [x] 10.4 增加管理员 Web3 充值工作台，默认突出 manual review 和 failed。
- [x] 10.5 验证管理员不能修改链上金额、用户、Token、tx hash 或 finalized 事实。

## 11. 可观测性和运行手册

- [x] 11.1 增加 RPC、scanner、finalizer、credit 和地址分配结构化日志。
- [x] 11.2 增加链高度延迟、状态堆积、重试、孤块和入账失败指标。
- [x] 11.3 增加 scanner 停滞、无健康 RPC、manual review 堆积和 credit failed 告警。
- [x] 11.4 编写密钥恢复、RPC 切换、补扫、人工审核和功能关闭运行手册。
- [x] 11.5 为第二期归集记录 `wallet_id + derivation_index` 恢复验证证据。

## 12. 灰度与上线

- [ ] 12.1 migration 上线后先保持功能关闭。
- [ ] 12.2 启用只观察 scanner，不启用自动 credit。
- [ ] 12.3 使用真实小额 USDT0 完成检测、finalized 和链上对账。
- [ ] 12.4 启用管理员灰度地址和事务性入账。
- [ ] 12.5 完成重启、RPC 故障、重复扫描和大额审核演练。
- [ ] 12.6 全量开放用户充值入口并记录上线基线指标。

## 当前状态备注

- 代码侧已实现并接线：配置默认关闭、HD 地址分配、钱包 fingerprint runtime 健康门禁、Conflux eSpace USDT0 RPC 验证、scanner/finalizer、Web3 子账户入账与用户幂等划转、持久化可恢复的受限补扫任务、管理审核审计、runtime scanner/finalizer 延迟、包含手续费/最终性说明的公开配置、自动获取地址的用户充值页与充值历史页面。
- 已执行验证：`go test ./internal/web3deposit/... ./internal/handler/... ./internal/repository/... ./migrations/... -run 'Web3|web3|Deposit' -count=1`；`go test ./internal/handler/... ./internal/server/middleware/... -run 'Web3|web3|Audit' -count=1`；`go test ./internal/web3deposit/... ./internal/handler/admin -run 'Web3|web3|ScannerFinalizer|Audit' -count=1`；`pnpm test:run web3Deposit admin.web3Deposits`；`pnpm typecheck`。
- 仍需补证或补实现：生产 HD wallet 离线基线、migration 典型查询 `EXPLAIN`、真实链小额灰度和全量上线基线。
