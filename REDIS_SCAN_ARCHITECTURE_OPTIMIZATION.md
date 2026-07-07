# Redis SCAN 架构优化执行文档

本文是架构级执行文档，只覆盖三个目标：

1. 账号并发活跃负载查询不再通过 Redis keyspace `SCAN` 发现账号。
2. 账号/用户并发槽过期清理、启动遗留槽清理不再通过 Redis keyspace `SCAN` 发现 key。
3. 用户消息队列孤儿锁清理不再通过 Redis keyspace `SCAN` 发现 lock key。

不覆盖旁路录制、OpenAI failover、日志量、业务限流配置调参。不要把本文扩展成短期止血方案。

## 成功标准

实现完成后必须同时满足：

- `backend/internal/repository/concurrency_cache.go` 中不得再调用 `c.rdb.Scan(...)`。
- `backend/internal/repository/user_msg_queue_cache.go` 中不得再调用 `c.rdb.Scan(...)`。
- `backend/internal/service/user_msg_queue_service.go` 中不得再出现 `ScanLockKeys` 接口调用。
- `GetActiveAccountLoadMap` 只读显式维护的 Redis 索引，不扫描 Redis keyspace。
- `CleanupExpiredAccountSlotKeys` 只处理显式索引中的候选账号，不扫描 Redis keyspace。
- `CleanupStaleProcessSlots` 不扫描 Redis keyspace；它必须基于显式索引清理，或只依赖 TTL/score 自然过期。
- UMQ cleanup worker 只读 `umq:lock:index` 候选项，不扫描 `umq:{*REDACTED:lock`。
- 主业务并发限制仍以原账号/用户 slot key 为准，不能以索引为准。索引只能用于发现候选对象、监控和清理。

执行完必须用下面命令确认生产代码没有遗留扫描：

```powershell
rg -n "\.Scan\(" backend/internal/repository/concurrency_cache.go backend/internal/repository/user_msg_queue_cache.go backend/internal/service/user_msg_queue_service.go
rg -n "ScanLockKeys|scanAccountIDsByPrefix|cleanupSlotsByPattern|deleteKeysByPattern|umqScanPattern" backend/internal/repository/concurrency_cache.go backend/internal/repository/user_msg_queue_cache.go backend/internal/service/user_msg_queue_service.go
```

第一条必须无输出。第二条必须无生产函数残留；测试文件不在此检查范围。

## 不可违反的约束

- 不要用 `KEYS` 替代 `SCAN`。
- 不要把全量 Redis keyspace 扫描移动到另一个函数、goroutine、启动流程或管理接口里。
- 不要在请求路径、后台 worker、启动流程中做 Redis keyspace pattern enumeration。
- 不要在 Redis Lua 脚本里同时操作“全局索引 key”和“账号/用户局部 key”。项目代码当前有 Redis Cluster 兼容要求，这种写法会在 Cluster 下触发 CROSSSLOT。
- 索引更新失败不得改变主业务 acquire/release 的成功结果。索引是 best-effort discovery structure，不是并发正确性的来源。
- 不能因为索引缺失而拒绝用户请求。索引缺失最多影响 Ops 实时视图和后台提前清理；原 slot/wait key 的 TTL 必须保证最终自愈。

## 新增 Redis Key

### 并发索引

在 `backend/internal/repository/concurrency_cache.go` 增加常量：

```go
const (
	accountActiveIndexKey = "concurrency:account:active_index" // ZSET member=accountID, score=expireAtUnixSeconds
	userActiveIndexKey    = "concurrency:user:active_index"    // ZSET member=userID, score=expireAtUnixSeconds

	activeIndexCleanupBatchSize  = 1000
	activeIndexPipelineChunkSize = 500
)
```

语义：

- `accountActiveIndexKey` 记录“可能有账号槽位或账号等待计数”的账号 ID。
- `userActiveIndexKey` 记录“可能有用户槽位或用户等待计数”的用户 ID。
- ZSET score 是候选对象的保守过期时间，单位为 Unix 秒。
- member 必须是十进制 ID 字符串，不要存完整 Redis key。
- 索引允许短暂 stale；读索引后必须二次查询真实 slot/wait key。

score 规则：

- 成功获取账号槽位：score = Redis 当前秒 + `slotTTLSeconds`。
- 成功增加账号等待计数：score = Redis 当前秒 + `waitQueueTTLSeconds`。
- 成功获取用户槽位：score = Redis 当前秒 + `slotTTLSeconds`。
- 成功增加用户等待计数：score = Redis 当前秒 + `waitQueueTTLSeconds`。
- release/decrement 后如果真实 slot count 和 wait count 都为 0，则从索引 `ZREM`。
- release/decrement 后如果仍有 slot 或 wait，则重新 `ZADD` 一个新的保守过期时间。

### UMQ 锁索引

在 `backend/internal/repository/user_msg_queue_cache.go` 增加常量：

```go
const (
	umqLockIndexKey = "umq:lock:index" // ZSET member=accountID, score=lockExpireAtUnixMs
	umqLockIndexCleanupBatchSize = 1000
)
```

语义：

- `umqLockIndexKey` 记录“可能存在 UMQ lock”的账号 ID。
- ZSET score 是 lock 的预计过期时间，单位为 Unix 毫秒。
- member 必须是十进制 accountID 字符串。
- 索引只用于 cleanup worker 找候选 lock。锁是否存在、是否孤儿，必须再查 `umq:{accountIDREDACTED:lock`。

## 第一部分：并发活跃索引

修改文件：`backend/internal/repository/concurrency_cache.go`。

### 1.1 增加 Redis 时间 helper

新增 helper，所有索引 score 使用 Redis server time，不用本机时间：

```go
func (c *concurrencyCache) redisUnixSeconds(ctx context.Context) (int64, error) {
	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return 0, fmt.Errorf("redis TIME: %w", err)
REDACTED
	return now.Unix(), nil
REDACTED
```

不要在 Lua 脚本里写全局索引，避免 CROSSSLOT。

### 1.2 增加索引 touch/remove/refresh helper

新增以下 helper。名字可以微调，但行为不能改。

```go
func (c *concurrencyCache) touchAccountActiveIndex(ctx context.Context, accountID int64, ttlSeconds int) {
	c.touchActiveIndex(ctx, accountActiveIndexKey, accountID, ttlSeconds)
REDACTED

func (c *concurrencyCache) touchUserActiveIndex(ctx context.Context, userID int64, ttlSeconds int) {
	c.touchActiveIndex(ctx, userActiveIndexKey, userID, ttlSeconds)
REDACTED

func (c *concurrencyCache) touchActiveIndex(ctx context.Context, indexKey string, id int64, ttlSeconds int) {
	if c == nil || c.rdb == nil || id <= 0 || ttlSeconds <= 0 {
		return
REDACTED
	now, err := c.redisUnixSeconds(ctx)
	if err != nil {
		return
REDACTED
	_ = c.rdb.ZAdd(ctx, indexKey, redis.Z{
		Score:  float64(now + int64(ttlSeconds)),
		Member: strconv.FormatInt(id, 10),
REDACTED).Err()
REDACTED
```

索引维护是 best-effort，所以 helper 内部吞掉错误。不要把索引错误返回给 acquire/release 调用方。

再新增 refresh helper：

```go
func (c *concurrencyCache) refreshAccountActiveIndex(ctx context.Context, accountID int64) {
	// 真实状态以 accountSlotKey(accountID) 和 accountWaitKey(accountID) 为准。
	// 先清理该账号 slot 中过期成员，再读 ZCARD 和 GET wait。
	// 如果 slotCount == 0 && waitCount <= 0：ZREM accountActiveIndexKey accountID。
	// 否则：ZADD accountActiveIndexKey accountID，score = now + maxRelevantTTL。
REDACTED

func (c *concurrencyCache) refreshUserActiveIndex(ctx context.Context, userID int64) {
	// 真实状态以 userSlotKey(userID) 和 waitQueueKey(userID) 为准。
	// 行为同 refreshAccountActiveIndex。
REDACTED
```

实现要求：

- `refresh*` 必须 best-effort，不能向 release/decrement 返回索引错误。
- `waitCount` 读取 `redis.Nil` 时按 0 处理。
- `waitCount < 0` 必须按 0 处理。
- `slotCount > 0` 时 score 至少延长 `slotTTLSeconds`。
- `waitCount > 0` 时 score 至少延长 `waitQueueTTLSeconds`。
- 两者都存在时使用更大的 TTL。

### 1.3 修改账号写路径

修改 `AcquireAccountSlot`：

```go
result, err := acquireScript.Run(...).Int()
if err != nil { return false, err REDACTED
if result == 1 {
	c.touchAccountActiveIndex(ctx, accountID, c.slotTTLSeconds)
REDACTED
return result == 1, nil
```

修改 `ReleaseAccountSlot`：

```go
if err := c.rdb.ZRem(ctx, key, requestID).Err(); err != nil {
	return err
REDACTED
c.refreshAccountActiveIndex(ctx, accountID)
return nil
```

修改 `IncrementAccountWaitCount`：

```go
result, err := incrementAccountWaitScript.Run(...).Int()
if err != nil { return false, err REDACTED
if result == 1 {
	c.touchAccountActiveIndex(ctx, accountID, c.waitQueueTTLSeconds)
REDACTED
return result == 1, nil
```

修改 `DecrementAccountWaitCount`：

```go
_, err := decrementWaitScript.Run(...).Result()
if err == nil {
	c.refreshAccountActiveIndex(ctx, accountID)
REDACTED
return err
```

### 1.4 修改用户写路径

同账号路径，修改：

- `AcquireUserSlot`
- `ReleaseUserSlot`
- `IncrementWaitCount`
- `DecrementWaitCount`

用户索引使用 `userActiveIndexKey`。

### 1.5 重写 GetActiveAccountLoadMap

删除 `scanAccountIDsByPrefix` 和 `parseAccountIDFromPrefixedKey` 的生产调用。`GetActiveAccountLoadMap` 必须改成：

1. 获取 Redis 当前秒。
2. `ZRemRangeByScore(accountActiveIndexKey, "-inf", strconv.FormatInt(now, 10))` 删除过期候选。
3. `ZRangeByScore(accountActiveIndexKey, &redis.ZRangeBy{Min: strconv.FormatInt(now+1, 10), Max: "+inf"REDACTED)` 获取候选账号 ID。
4. 解析 member 为 `int64`，非法 member 记录到待删除列表。
5. 分块 pipeline，块大小 `activeIndexPipelineChunkSize`。
6. 对每个候选账号执行：
   - `ZRemRangeByScore(accountSlotKey(id), "-inf", cutoffUnixSeconds)`
   - `ZCard(accountSlotKey(id))`
   - `Get(accountWaitKey(id))`
7. 构造结果时只返回 `currentConcurrency > 0 || waitingCount > 0` 的账号。
8. 对真实状态为空或 member 非法的账号执行 `ZREM accountActiveIndexKey member`。
9. 对真实状态仍活跃但 index score 已接近过期的账号，调用 `touchAccountActiveIndex` 刷新。

禁止：

- 禁止再扫 `concurrency:account:*`。
- 禁止再扫 `wait:account:*`。
- 禁止用索引里的 score 直接判断并发数。

## 第二部分：并发槽清理和启动清理

修改文件：`backend/internal/repository/concurrency_cache.go`。

### 2.1 重写 CleanupExpiredAccountSlotKeys

当前实现调用 `cleanupExpiredSlotKeysByPattern(ctx, accountSlotKeyPrefix+"*")`，必须删除。

新行为：

1. 获取 Redis 当前秒 `now`。
2. 从 `accountActiveIndexKey` 读取过期候选：

```go
ids, err := c.rdb.ZRangeByScore(ctx, accountActiveIndexKey, &redis.ZRangeBy{
	Min: "-inf",
	Max: strconv.FormatInt(now, 10),
	Count: activeIndexCleanupBatchSize,
REDACTED).Result()
```

3. 对每个候选账号清理该账号 slot 过期成员并读真实状态。
4. 如果真实 `slotCount == 0 && waitCount <= 0`，从 `accountActiveIndexKey` 删除该账号。
5. 如果真实仍活跃，刷新 `accountActiveIndexKey` score。
6. 不需要处理不在索引中的账号；其 slot key 自身有 `EXPIRE`，并且 acquire/get-load 会惰性清理过期成员。

这个函数不再表示“遍历所有账号槽位 key”，而是“处理索引中到期的账号候选”。保留原函数名是为了少改接口。

### 2.2 重写 CleanupStaleProcessSlots

当前实现会扫描：

- `concurrency:account:*`
- `concurrency:user:*`
- `wait:account:*`
- `concurrency:wait:*`

必须去掉这些扫描。

新行为必须基于索引：

1. 从 `accountActiveIndexKey` 读取所有未过期候选账号。
2. 对每个账号：
   - 对 `accountSlotKey(id)` 运行“单 key 清理脚本”，删除 requestID 前缀不是当前 `activeRequestPrefix` 的成员。
   - 删除 `accountWaitKey(id)`，因为等待者属于旧进程，重启后不能继续等待。
   - 调用 `refreshAccountActiveIndex(ctx, id)`。
3. 从 `userActiveIndexKey` 读取所有未过期候选用户。
4. 对每个用户：
   - 对 `userSlotKey(id)` 运行同一个“单 key 清理脚本”。
   - 删除 `waitQueueKey(id)`。
   - 调用 `refreshUserActiveIndex(ctx, id)`。

新增单 key Lua 脚本，替代当前 `startupCleanupScript` 的多 key 版本：

```lua
local key = KEYS[1]
local activePrefix = ARGV[1]
local slotTTL = tonumber(ARGV[2])
local removed = 0
local members = redis.call('ZRANGE', key, 0, -1)
for _, member in ipairs(members) do
  if string.sub(member, 1, string.len(activePrefix)) ~= activePrefix then
    removed = removed + redis.call('ZREM', key, member)
  end
end
if redis.call('ZCARD', key) == 0 then
  redis.call('DEL', key)
else
  redis.call('EXPIRE', key, slotTTL)
end
return removed
```

该脚本只接受一个 slot key，避免 Redis Cluster CROSSSLOT。

如果索引不存在或为空：

- `CleanupStaleProcessSlots` 直接返回 nil。
- 不要 fallback 到 `SCAN`。
- 旧版本遗留 key 依赖 Redis TTL 自然过期。不要在 app 启动时做兼容性 keyspace backfill。

### 2.3 删除旧扫描函数

删除以下生产函数：

- `scanAccountIDsByPrefix`
- `parseAccountIDFromPrefixedKey`，如果没有其他生产调用
- `cleanupExpiredSlotKeysByPattern`
- `cleanupSlotsByPattern`
- `deleteKeysByPattern`

如果测试需要解析 key，测试内自建 helper，不要保留生产 helper。

## 第三部分：UMQ 锁索引

修改文件：

- `backend/internal/repository/user_msg_queue_cache.go`
- `backend/internal/service/user_msg_queue_service.go`

### 3.1 修改 service 接口

在 `backend/internal/service/user_msg_queue_service.go` 的 `UserMsgQueueCache` 接口中删除：

```go
ScanLockKeys(ctx context.Context, maxCount int) ([]int64, error)
ForceReleaseLock(ctx context.Context, accountID int64) error
```

替换为：

```go
ReconcileExpiredLockCandidates(ctx context.Context, maxCount int) (cleaned int, err error)
```

原因：cleanup worker 不应该知道 Redis lock key 的枚举方式，也不应该先枚举再逐个 `ForceReleaseLock`。候选读取、PTTL 校验、索引刷新应该封装在 cache 层。

### 3.2 修改 acquireLockScript 返回值

当前脚本只返回 0/1。改成返回数组：

```lua
redis.replicate_commands()
local cur = redis.call('GET', KEYS[1])
local ttl = tonumber(ARGV[2])
if cur == ARGV[1] then
  redis.call('PEXPIRE', KEYS[1], ttl)
  local t = redis.call('TIME')
  local ms = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
  return {1, ms + ttlREDACTED
end
if cur ~= false then
  return {0, 0REDACTED
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ttl)
local t = redis.call('TIME')
local ms = tonumber(t[1])*1000 + math.floor(tonumber(t[2])/1000)
return {1, ms + ttlREDACTED
```

Go 侧解析：

- 第一个元素是 acquired，1 表示拿到锁。
- 第二个元素是 Redis 时间计算出的 `expireAtUnixMs`。
- acquired 为 1 时，best-effort 写 `ZADD umqLockIndexKey expireAtMs accountID`。
- `ZADD` 失败不能让 `AcquireLock` 返回失败。

### 3.3 修改 ReleaseLock

`ReleaseLock` 主逻辑保持原子释放锁和写 last key。

释放成功时：

```go
if result == 1 {
	_ = c.rdb.ZRem(ctx, umqLockIndexKey, strconv.FormatInt(accountID, 10)).Err()
REDACTED
```

释放失败时不要删除索引。失败可能是 requestID 不匹配或 lock 已过期；cleanup worker 会处理 stale index。

### 3.4 新增 reconcile 脚本

删除 `forceReleaseLockScript` 的外部使用。新增脚本：

```lua
local pttl = redis.call('PTTL', KEYS[1])
if pttl == -2 then
  return {-2, 0REDACTED
end
if pttl == -1 then
  redis.call('DEL', KEYS[1])
  return {-1, 0REDACTED
end
return {1, pttlREDACTED
```

返回语义：

- `-2`：lock key 不存在。Go 侧 `ZREM umqLockIndexKey accountID`。
- `-1`：lock key 存在但无 TTL，脚本已删除。Go 侧 `ZREM umqLockIndexKey accountID`，cleaned++。
- `1`：lock key 仍有 TTL。Go 侧用 Redis 当前毫秒 + pttl 刷新 `umqLockIndexKey` score。

### 3.5 实现 ReconcileExpiredLockCandidates

实现步骤：

1. 用 `c.rdb.Time(ctx)` 获取 Redis 当前毫秒 `nowMs`。
2. 从 `umqLockIndexKey` 取到期候选：

```go
members, err := c.rdb.ZRangeByScore(ctx, umqLockIndexKey, &redis.ZRangeBy{
	Min: "-inf",
	Max: strconv.FormatInt(nowMs, 10),
	Count: int64(maxCount),
REDACTED).Result()
```

3. 逐个解析 accountID。非法 member 直接 `ZREM`。
4. 对合法 accountID 运行 reconcile 脚本，key 为 `umqLockKey(accountID)`。
5. 根据返回值删除索引、刷新索引或累计 cleaned。
6. 函数返回 cleaned 数。

禁止：

- 禁止 fallback 到 `SCAN umq:{*REDACTED:lock`。
- 禁止用 `KEYS umq:*`。
- 禁止 cleanup worker 自己解析 lock key。

### 3.6 修改 StartCleanupWorker

当前 worker 先 `ScanLockKeys` 再逐个 `ForceReleaseLock`。改成：

```go
cleaned, err := s.cache.ReconcileExpiredLockCandidates(ctx, 1000)
if err != nil {
	logger.LegacyPrintf("service.umq", "Cleanup reconcile failed: %v", err)
	return
REDACTED
if cleaned > 0 {
	logger.LegacyPrintf("service.umq", "Cleanup completed: released %d orphaned locks", cleaned)
REDACTED
```

worker 不再知道扫描、PTTL、索引等细节。

### 3.7 删除旧 UMQ 扫描函数

删除：

- `umqScanPattern`
- `ScanLockKeys`
- `ForceReleaseLock`，如果无生产调用

如果测试仍需要强造 PTTL == -1 的 key，只在测试里直接写 Redis。

## 测试要求

### 并发缓存测试

新增或修改 `backend/internal/repository/concurrency_cache_*_test.go`。

必须覆盖：

1. `AcquireAccountSlot` 成功后 `GetActiveAccountLoadMap` 能看到该账号。
2. `ReleaseAccountSlot` 后 `GetActiveAccountLoadMap` 不再返回该账号。
3. `IncrementAccountWaitCount` 成功后 `GetActiveAccountLoadMap` 能看到 waiting count。
4. `DecrementAccountWaitCount` 后如果无 slot，则索引被移除。
5. `CleanupExpiredAccountSlotKeys` 不依赖 keyspace scan：测试里只创建索引成员和对应 slot key，然后确认会清理；再创建未索引 slot key，确认不会被该函数主动发现。
6. `CleanupStaleProcessSlots` 只处理索引中的 account/user，删除旧 request prefix 成员，保留当前 prefix 成员，删除 account/user wait key。
7. 索引中存在非法 member 时，`GetActiveAccountLoadMap` 不报错，并移除非法 member。

### UMQ 测试

新增或修改 `backend/internal/repository/user_msg_queue_cache*_test.go` 和 `backend/internal/service/user_msg_queue_service*_test.go`。

必须覆盖：

1. `AcquireLock` 成功后写入 `umq:lock:index`，score 大于 Redis 当前毫秒。
2. `ReleaseLock` 成功后删除 `umq:lock:index` member。
3. lock 已自然过期时，`ReconcileExpiredLockCandidates` 删除 stale index member。
4. lock 仍有 TTL 但 index score 到期时，`ReconcileExpiredLockCandidates` 刷新 index score，不删除 lock。
5. lock 存在且 `PTTL == -1` 时，`ReconcileExpiredLockCandidates` 删除 lock，删除 index member，并返回 cleaned=1。
6. index 中非法 member 不导致错误，并被删除。
7. `StartCleanupWorker` 调用 `ReconcileExpiredLockCandidates`，不再调用 `ScanLockKeys` 或 `ForceReleaseLock`。

### 禁止项测试

实现完成后运行：

```powershell
rg -n "\.Scan\(" backend/internal/repository/concurrency_cache.go backend/internal/repository/user_msg_queue_cache.go backend/internal/service/user_msg_queue_service.go
rg -n "ScanLockKeys|umqScanPattern|scanAccountIDsByPrefix|cleanupExpiredSlotKeysByPattern|cleanupSlotsByPattern|deleteKeysByPattern" backend/internal/repository/concurrency_cache.go backend/internal/repository/user_msg_queue_cache.go backend/internal/service/user_msg_queue_service.go
```

上述命令必须无输出。

再运行相关测试。按项目约定，编译很慢时先把代码复制到 WSL 文件系统再跑：

```bash
cd backend
go test ./internal/repository ./internal/service
```

如果全量包太慢，至少先跑：

```bash
cd backend
go test ./internal/repository -run 'Concurrency|UserMsgQueue|Redis'
go test ./internal/service -run 'Concurrency|UserMessageQueue'
```

## 迁移和兼容

不要在应用启动时扫描旧 key 回填索引。

原因：

- 这会把问题从运行期 `SCAN` 搬到启动期 `SCAN`。
- 生产实例重启时 Redis 已经高 CPU，启动扫描会放大抖动。
- 并发 slot key 和 wait key 都有 TTL，新版本写路径会为新流量维护索引，旧 key 可自然过期。

兼容策略：

- 新版本上线后，新请求会逐步填充 `concurrency:*:active_index` 和 `umq:lock:index`。
- 旧并发 slot key 没有索引时，不影响并发限制本身；对应账号下一次 acquire/get-load 会清理自己的 slot。
- 旧 UMQ lock 如果有 TTL，会自然过期。
- 极少数历史 `PTTL == -1` UMQ lock 且没有 index 的情况，不由应用自动发现。需要人工离线维护时，单独写一次性脚本，维护窗口运行，不要放进服务启动或后台 worker。

## 代码审查检查表

提交前逐项确认：

- [ ] 没有新增 `KEYS`。
- [ ] 没有新增生产路径 `SCAN`。
- [ ] 没有在 Lua 脚本中同时操作全局索引 key 和账号/用户局部 key。
- [ ] 索引维护失败不会让 acquire/release/decrement 的主结果失败。
- [ ] `GetActiveAccountLoadMap` 对 stale index、非法 member、Redis nil 都能正常返回。
- [ ] `CleanupExpiredAccountSlotKeys` 不再遍历 keyspace。
- [ ] `CleanupStaleProcessSlots` 不再遍历 keyspace。
- [ ] UMQ cleanup worker 不再知道 lock key pattern。
- [ ] 所有旧扫描 helper 已删除或仅存在于测试文件。
- [ ] 新测试覆盖成功路径、stale index、非法 member、PTTL -1、自然过期。
