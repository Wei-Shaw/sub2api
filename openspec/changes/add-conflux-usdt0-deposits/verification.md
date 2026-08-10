# Web3 Deposit 验证计划

## 1. P0 安全门禁

以下任一失败都禁止上线：

- 生产主服务配置、数据库、日志和 API 中不存在助记词、根私钥或子私钥。
- xpub 能通过离线基线测试重新派生数据库中的样本地址。
- RPC chain ID 必须为 `1030`，Token code 非空，decimals 为 6，支持 finalized tag。
- Token 合约只接受固定官方地址；OFT 或同名 Token 不能产生充值。
- raw amount、token amount 和 credited amount 全程不经过 `float64`。
- 同一 `(chain_id, tx_hash, log_index)` 无论重复发现、并发履约或重启多少次，只增加一次余额。
- 未 finalized、验证失败、orphaned、低于最低金额和未批准大额充值不能增加余额。
- 充值与 Web3 子账户入账保持原子一致；Web3 向主余额的划转、`total_recharged` 和 transfer 事实保持原子一致。

## 2. HD 钱包验证

### 2.1 测试向量

固定一个仅用于测试的账户级 xpub，至少验证 index `0`、`1`、`2`、`1000` 的：

```text
compressed public key
uncompressed public key
lowercase EVM address
checksum EVM address
```

结果必须与至少一个独立钱包实现离线对照。

### 2.2 并发

- 同一用户 100 个并发 POST，只返回一个 address ID/index/address。
- 100 个不同用户并发分配，index 不重复，地址不重复。
- 插入提交前进程退出允许 index 空洞，恢复后不得重用。
- `wallet_id` 对应 fingerprint 变化时，新地址分配全部失败，历史地址查询仍可用。

### 2.3 敏感信息

搜索应用日志、测试输出、API JSON 和数据库普通列：

```text
xprv
助记词 canary
测试子私钥
完整生产 xpub canary
```

期望 0 个非测试夹具泄露。失败消息只能输出 wallet ID、index 和 fingerprint。

## 3. ERC20 解析验证

覆盖：

- 标准 Transfer，6 位金额正确。
- 一笔交易中多个 Transfer、同一 to 两个 log index。
- from/to topic padding 错误。
- topic 数量不足或 data 非 32 字节。
- raw amount 为 0。
- 最大 uint256 和超过平台可表示范围。
- Token 合约不匹配。
- chain ID 不匹配。
- removed log。
- 大小写不同的合法地址映射。

解析器不得因为 malformed log panic。

## 4. Scanner 与游标验证

### 4.1 批次原子性

- 批次中途数据库失败：cursor 不推进；已插入日志在重试时被唯一键去重。
- RPC timeout：cursor 不推进。
- malformed log：批次失败并告警，不静默跳过。
- 空区块范围：允许推进 cursor。

### 4.2 重扫与多实例

- overlap 区间重复返回相同日志，不增加重复充值。
- 两个实例同时启动，只有 leader 推进 cursor。
- leader 在批次中失去租约，不能以旧租约提交新 cursor。
- leader 退出后另一实例从数据库 cursor 接管。
- 从相同 start block 全量重放，充值集合与首次扫描一致。

### 4.3 RPC failover

- 主 RPC 超时后切换健康备用节点。
- 备用节点 chain ID 错误时被隔离。
- 所有节点 unhealthy 时 scanner 停止推进并暴露 degraded/unhealthy。
- 恢复后从原 cursor 继续，不跳块。

## 5. Finality 和重组验证

- 区块高于 finalized：状态保持 confirming，不入账。
- stored block hash 与 canonical hash 不同：标记 orphaned。
- receipt 不存在、失败或 block hash 不一致：不入账。
- receipt 中目标 log 消失或金额变化：不入账并告警。
- finalized 验证成功且金额在自动范围：进入 ready_to_credit。
- finalized 验证成功且金额大于 10,000：进入 manual_review。
- finalized 验证成功且金额小于 1：进入 below_minimum。
- 正好 1 和正好 10,000 自动入账；只有大于 10,000 才人工审核。

## 6. 余额原子性与幂等

### 6.1 并发履约

准备一条 `ready_to_credit` 充值，启动 100 个 goroutine/两个 service 实例并发调用 credit：

```text
balance increase count = 1
total_recharged increase count = 1
credited deposit count = 1
deposit status = credited
```

### 6.2 故障注入

分别在以下位置注入错误：

1. 锁定充值后；
2. 更新 Web3 子账户前；
3. 更新 Web3 子账户后；
4. 更新用户余额后；
5. 更新充值状态前；
6. commit 前；
7. commit 后缓存失效前；
8. commit 后通知发送前。

前六项必须完整回滚或可安全重试；后两项余额不得回滚，重试不得重复入账。

### 6.3 精度

至少验证：

```text
1 raw unit       -> 0.000001 USD
1 USDT0          -> 1.000000 USD
123.456789       -> exact same USD
9999.999999      -> auto credit
10000.000000     -> auto credit
10000.000001     -> manual review
```

数据库读取、Web3 子账户和划转快照必须逐位相同。

## 7. 用户状态验证

- active 用户按正常规则入账。
- disabled/banned/soft-deleted 用户进入 manual review。
- 地址仍能匹配已删除用户，不丢弃日志。
- 用户不存在但地址 FK/数据异常时停止入账并触发高优先级告警。
- 用户只能查询自己的地址和充值详情；枚举其他 ID 返回 not found/forbidden，不泄露事实。

## 8. 管理审核验证

- approve 只接受 manual_review。
- approve 前必须重新执行 finalized canonical verification。
- 两个管理员并发 approve 仍只入账一次。
- ignore 必须填写原因并二次确认。
- retry 只接受允许重试的状态。
- 所有写操作记录管理员、充值 ID、旧状态、新状态、原因和时间。
- 管理员不能覆盖金额、用户、tx hash、log index、block hash 或 Token。

## 9. API 契约验证

### 用户端

- 功能关闭时 config 明确 disabled，POST address 不分配。
- 页面初始化自动调用 POST address，不需要用户点击创建按钮。
- POST address 幂等，重复进入页面返回原地址且不消耗新的 derivation index。
- 金额字段为 JSON string。
- config 返回字符串手续费和最终性说明。
- 地址和 Token 合约同时展示。
- 列表分页稳定，按 `created_at,id` 排序无重复遗漏。
- 内部 `crediting/ready_to_credit` 不直接暴露。

### 管理端

- 未认证、非管理员和缺少 step-up 的写请求被拒绝。
- tx hash/address 搜索大小写不敏感。
- runtime 返回 endpoint ID、健康、leader 和高度，不返回完整 xpub 或带凭据 RPC URL。
- bounded rescan 拒绝过大区间、未来区块和低于允许下界的请求。
- bounded rescan 返回持久任务，任务列表和详情可查询状态、计数、错误与时间戳。
- 补扫任务的 `from_block` 和 `to_block` 使用 JSON string，超出 JavaScript 安全整数时仍保持精确。
- 运行中断或租约过期后任务可被重新领取，旧尝试不能覆盖新尝试结果。

## 10. 前端验证

- 用户页面明确显示 `Conflux eSpace`，不得只显示 `Conflux`。
- 明确显示 USDT0 Token 合约并提供复制。
- 显示最低 1 USDT0、大额审核和 finalized 后到账提示。
- 警告 Core Space、错误网络和其他同名 Token 不自动处理。
- 二维码内容只包含分配地址。
- confirming、credited、below minimum、under review、failed 均有可理解文案。
- 移动端地址和 tx hash 不溢出；复制按钮键盘可达。
- 页面卸载后停止 polling。

## 11. 运行时与可观测性

- Start/Stop 重复调用安全，shutdown 在超时内完成。
- scanner panic 被捕获并使 runtime unhealthy。
- latest/finalized/scanned block 和 lag 指标随运行更新。
- 状态堆积、RPC 全部失败、cursor 停滞和 credit failed 能触发告警。
- 日志使用稳定字段，不输出 raw RPC body 中潜在凭据。

## 12. 真实链灰度验收

在生产参数但受限用户范围中完成：

1. 创建测试用户地址并从离线种子重新派生核对。
2. 发送一笔 `1.xxxxxx` USDT0。
3. 对照 Conflux 浏览器和两个 RPC 节点确认 tx hash、log index、block hash 和 raw amount。
4. 用户页面先显示 confirming。
5. finalized 后只有一次 Web3 子账户入账，余额精确增加。
6. 重启全部应用实例并补扫包含该交易的区间，余额不再变化。
7. 发送一笔低于 1 USDT0，确认不入账。
8. 在测试或受控环境构造大于 10,000 的等价数据，完成管理员批准流程；不得为测试实际发送大额生产资金。

## 13. 建议验证命令

实施后根据实际包名调整：

```bash
cd backend
go test ./internal/web3deposit/...
go test ./internal/repository/... ./internal/handler/... ./internal/server/...
go test ./migrations/...
go test ./...

cd ../frontend
npm run type-check
npm run test
npm run build
```

若仓库实际脚本名称不同，以 `package.json` 和 Makefile 为准，不新增重复脚本。
