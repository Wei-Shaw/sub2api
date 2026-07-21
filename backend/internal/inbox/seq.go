package inbox

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// seqRedisKey 是全局 seq 分配器在 Redis 中的哈希键。字段：
//
//	ms  -> 已分配过的最大毫秒时间戳（单调，永不回退）
//	cnt -> 该毫秒内已分配计数
const seqRedisKey = "inbox:seq:global"

// seqCounterMax 是单毫秒内计数器的上限（2^SeqTimestampShift）。达到后进位到下一毫秒。
const seqCounterMax = 1 << SeqTimestampShift

// seqAllocScript 原子分配下一个 (ms, cnt)。
//
// 关键性质：
//   - ms 取 wall clock 与已存最大值的较大者，保证时间回拨时 seq 仍单调递增；
//   - 同一毫秒内用 HINCRBY 递增 cnt，溢出（>= maxcnt）时进位到 ms+1 并重置 cnt=0；
//   - 全程在单条 Lua 中执行，天然串行、无竞态。
//
// 返回 {ms, cnt}，由 Go 侧组合成 seq = ms<<SeqTimestampShift | cnt。
var seqAllocScript = redis.NewScript(`
local key = KEYS[1]
local maxcnt = tonumber(ARGV[1])
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local last = tonumber(redis.call('HGET', key, 'ms') or '0')
local ms
local cnt
if now > last then
  ms = now
  cnt = 0
  redis.call('HSET', key, 'ms', ms, 'cnt', cnt)
else
  ms = last
  cnt = redis.call('HINCRBY', key, 'cnt', 1)
  if cnt >= maxcnt then
    ms = last + 1
    cnt = 0
    redis.call('HSET', key, 'ms', ms, 'cnt', cnt)
  end
end
return {ms, cnt}
`)

// SeqSource 分配全局单调递增的 seq。SeqAllocator 是其 Redis 实现；抽象为接口便于
// Publisher 在单测中注入 fake。
type SeqSource interface {
	Next(ctx context.Context) (int64, error)
}

// SeqAllocator 基于 Redis Lua 脚本分配全局单调 seq。脚本通过 EVALSHA/EVAL 自动加载
// （go-redis Script.Run 会在 NOSCRIPT 时回退到 EVAL）。
type SeqAllocator struct {
	rdb *redis.Client
}

// NewSeqAllocator 构造基于给定 Redis 客户端的 seq 分配器。
func NewSeqAllocator(rdb *redis.Client) *SeqAllocator {
	return &SeqAllocator{rdb: rdb}
}

// Next 分配下一个 seq。返回值严格大于此前所有已分配的 seq（同一 Redis 实例内）。
func (a *SeqAllocator) Next(ctx context.Context) (int64, error) {
	if a == nil || a.rdb == nil {
		return 0, ErrSeqAllocFailed.WithCause(fmt.Errorf("seq allocator 未初始化"))
	}
	res, err := seqAllocScript.Run(ctx, a.rdb, []string{seqRedisKey}, seqCounterMax).Result()
	if err != nil {
		return 0, ErrSeqAllocFailed.WithCause(err)
	}
	arr, ok := res.([]any)
	if !ok || len(arr) != 2 {
		return 0, ErrSeqAllocFailed.WithCause(fmt.Errorf("seq 脚本返回非预期结果: %v", res))
	}
	ms, ok1 := arr[0].(int64)
	cnt, ok2 := arr[1].(int64)
	if !ok1 || !ok2 {
		return 0, ErrSeqAllocFailed.WithCause(fmt.Errorf("seq 脚本返回类型非 int64: %v", res))
	}
	return ms<<SeqTimestampShift | cnt, nil
}
