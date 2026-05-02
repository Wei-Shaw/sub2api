// redis_client_collections.go carries the hash / list / set / sorted-set /
// server / scripting / pub-sub typed wrappers around RedisClient.Do.
// Split out of redis_client_typed.go (T46) so each typed file fits
// under the 500-line CLAUDE.md threshold.

package driver

import (
	"context"
	"errors"
	"fmt"
	"io"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// ============================================================
// Cmdable — Hash commands
// ============================================================

func (r *RedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	cmd := r.Do(ctx, "HGET", []int{0}, key, field).AsString()
	return cmd.val, cmd.err
}

func (r *RedisClient) HSet(ctx context.Context, key, field, value string) error {
	return r.Do(ctx, "HSET", []int{0}, key, field, value).AsInt().Err()
}

func (r *RedisClient) HDel(ctx context.Context, key string, fields ...string) error {
	if len(fields) == 0 {
		return nil
	}
	args := append([]any{key}, anySlice(fields)...)
	return r.Do(ctx, "HDEL", []int{0}, args...).AsInt().Err()
}

// HGetAll fetches all fields of a hash.
func (r *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	cmd := r.Do(ctx, "HGETALL", []int{0}, key).AsStringStringMap()
	return cmd.val, cmd.err
}

func (r *RedisClient) HExists(ctx context.Context, key, field string) *BoolCmd {
	return r.Do(ctx, "HEXISTS", []int{0}, key, field).AsBool()
}

func (r *RedisClient) HKeys(ctx context.Context, key string) *StringSliceCmd {
	return r.Do(ctx, "HKEYS", []int{0}, key).AsStringSlice()
}

func (r *RedisClient) HVals(ctx context.Context, key string) *StringSliceCmd {
	return r.Do(ctx, "HVALS", []int{0}, key).AsStringSlice()
}

func (r *RedisClient) HLen(ctx context.Context, key string) *IntCmd {
	return r.Do(ctx, "HLEN", []int{0}, key).AsInt()
}

func (r *RedisClient) HIncrBy(ctx context.Context, key, field string, incr int64) *IntCmd {
	return r.Do(ctx, "HINCRBY", []int{0}, key, field, incr).AsInt()
}

func (r *RedisClient) HIncrByFloat(ctx context.Context, key, field string, incr float64) *FloatCmd {
	return r.Do(ctx, "HINCRBYFLOAT", []int{0}, key, field, incr).AsFloat()
}

func (r *RedisClient) HMGet(ctx context.Context, key string, fields ...string) *SliceCmd {
	args := append([]any{key}, anySlice(fields)...)
	return r.Do(ctx, "HMGET", []int{0}, args...).AsSlice()
}

// HMSet (deprecated by Redis but still used) accepts a map.
func (r *RedisClient) HMSet(ctx context.Context, key string, fields map[string]any) *StatusCmd {
	args := make([]any, 0, 1+2*len(fields))
	args = append(args, key)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return r.Do(ctx, "HMSET", []int{0}, args...).AsStatus()
}

// ============================================================
// Cmdable — List commands
// ============================================================

func (r *RedisClient) LPush(ctx context.Context, key string, values ...any) *IntCmd {
	args := append([]any{key}, values...)
	return r.Do(ctx, "LPUSH", []int{0}, args...).AsInt()
}

func (r *RedisClient) RPush(ctx context.Context, key string, values ...any) *IntCmd {
	args := append([]any{key}, values...)
	return r.Do(ctx, "RPUSH", []int{0}, args...).AsInt()
}

func (r *RedisClient) LPop(ctx context.Context, key string) *StringCmd {
	return r.Do(ctx, "LPOP", []int{0}, key).AsString()
}

func (r *RedisClient) RPop(ctx context.Context, key string) *StringCmd {
	return r.Do(ctx, "RPOP", []int{0}, key).AsString()
}

func (r *RedisClient) LRange(ctx context.Context, key string, start, stop int64) *StringSliceCmd {
	return r.Do(ctx, "LRANGE", []int{0}, key, start, stop).AsStringSlice()
}

func (r *RedisClient) LLen(ctx context.Context, key string) *IntCmd {
	return r.Do(ctx, "LLEN", []int{0}, key).AsInt()
}

func (r *RedisClient) LIndex(ctx context.Context, key string, index int64) *StringCmd {
	return r.Do(ctx, "LINDEX", []int{0}, key, index).AsString()
}

func (r *RedisClient) LRem(ctx context.Context, key string, count int64, value any) *IntCmd {
	return r.Do(ctx, "LREM", []int{0}, key, count, value).AsInt()
}

func (r *RedisClient) LTrim(ctx context.Context, key string, start, stop int64) *StatusCmd {
	return r.Do(ctx, "LTRIM", []int{0}, key, start, stop).AsStatus()
}

// ============================================================
// Cmdable — Set commands
// ============================================================

func (r *RedisClient) SAdd(ctx context.Context, key string, members ...any) *IntCmd {
	args := append([]any{key}, members...)
	return r.Do(ctx, "SADD", []int{0}, args...).AsInt()
}

func (r *RedisClient) SRem(ctx context.Context, key string, members ...any) *IntCmd {
	args := append([]any{key}, members...)
	return r.Do(ctx, "SREM", []int{0}, args...).AsInt()
}

func (r *RedisClient) SMembers(ctx context.Context, key string) *StringSliceCmd {
	return r.Do(ctx, "SMEMBERS", []int{0}, key).AsStringSlice()
}

func (r *RedisClient) SIsMember(ctx context.Context, key string, member any) *BoolCmd {
	return r.Do(ctx, "SISMEMBER", []int{0}, key, member).AsBool()
}

func (r *RedisClient) SCard(ctx context.Context, key string) *IntCmd {
	return r.Do(ctx, "SCARD", []int{0}, key).AsInt()
}

func (r *RedisClient) SPop(ctx context.Context, key string) *StringCmd {
	return r.Do(ctx, "SPOP", []int{0}, key).AsString()
}

func (r *RedisClient) SRandMember(ctx context.Context, key string) *StringCmd {
	return r.Do(ctx, "SRANDMEMBER", []int{0}, key).AsString()
}

// ============================================================
// Cmdable — Sorted Set commands (subset; full surface fits the same pattern)
// ============================================================

// ZAddArgs models a Z member entry. Score + Member, mirroring go-redis.Z.
type ZAddArgs struct {
	Score  float64
	Member any
}

func (r *RedisClient) ZAdd(ctx context.Context, key string, members ...ZAddArgs) *IntCmd {
	args := make([]any, 0, 1+2*len(members))
	args = append(args, key)
	for _, m := range members {
		args = append(args, m.Score, m.Member)
	}
	return r.Do(ctx, "ZADD", []int{0}, args...).AsInt()
}

func (r *RedisClient) ZRem(ctx context.Context, key string, members ...any) *IntCmd {
	args := append([]any{key}, members...)
	return r.Do(ctx, "ZREM", []int{0}, args...).AsInt()
}

func (r *RedisClient) ZRange(ctx context.Context, key string, start, stop int64) *StringSliceCmd {
	return r.Do(ctx, "ZRANGE", []int{0}, key, start, stop).AsStringSlice()
}

func (r *RedisClient) ZRevRange(ctx context.Context, key string, start, stop int64) *StringSliceCmd {
	return r.Do(ctx, "ZREVRANGE", []int{0}, key, start, stop).AsStringSlice()
}

func (r *RedisClient) ZRangeByScore(ctx context.Context, key string, min, max string) *StringSliceCmd {
	return r.Do(ctx, "ZRANGEBYSCORE", []int{0}, key, min, max).AsStringSlice()
}

func (r *RedisClient) ZScore(ctx context.Context, key, member string) *FloatCmd {
	return r.Do(ctx, "ZSCORE", []int{0}, key, member).AsFloat()
}

func (r *RedisClient) ZIncrBy(ctx context.Context, key string, incr float64, member string) *FloatCmd {
	return r.Do(ctx, "ZINCRBY", []int{0}, key, incr, member).AsFloat()
}

func (r *RedisClient) ZRank(ctx context.Context, key, member string) *IntCmd {
	return r.Do(ctx, "ZRANK", []int{0}, key, member).AsInt()
}

func (r *RedisClient) ZCard(ctx context.Context, key string) *IntCmd {
	return r.Do(ctx, "ZCARD", []int{0}, key).AsInt()
}

func (r *RedisClient) ZCount(ctx context.Context, key, min, max string) *IntCmd {
	return r.Do(ctx, "ZCOUNT", []int{0}, key, min, max).AsInt()
}

// ============================================================
// Cmdable — Server commands
// ============================================================

// Ping issues PING. Returns "PONG" on success.
func (r *RedisClient) Ping(ctx context.Context) *StatusCmd {
	// PING has no key arguments.
	return r.Do(ctx, "PING", nil).AsStatus()
}

// ============================================================
// Cmdable — Scripting
// ============================================================

// Eval runs a Lua script. The script source is the first argument; numKeys
// must match the leading args that are keys (those get namespaced); the
// trailing args are pure values.
func (r *RedisClient) Eval(ctx context.Context, script string, keys []string, args ...any) *DoCmd {
	full := []any{script, len(keys)}
	positions := make([]int, 0, len(keys))
	for i, k := range keys {
		full = append(full, k)
		// position in args (excluding command) for this key
		positions = append(positions, 2+i)
	}
	full = append(full, args...)
	return r.Do(ctx, "EVAL", positions, full...)
}

// EvalSha runs a previously-loaded script by SHA1.
func (r *RedisClient) EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) *DoCmd {
	full := []any{sha1, len(keys)}
	positions := make([]int, 0, len(keys))
	for i, k := range keys {
		full = append(full, k)
		positions = append(positions, 2+i)
	}
	full = append(full, args...)
	return r.Do(ctx, "EVALSHA", positions, full...)
}

// ScriptLoad caches a script, returning its SHA1.
func (r *RedisClient) ScriptLoad(ctx context.Context, script string) *StringCmd {
	return r.Do(ctx, "SCRIPT", nil, "LOAD", script).AsString()
}

// ============================================================
// Pub/Sub — pass through unchanged (channels are not key-namespaced)
// ============================================================

// Publish broadcasts on a pub/sub channel. Channels are NOT namespaced —
// they carry the plugin name in payload schemas instead.
func (r *RedisClient) Publish(ctx context.Context, channel string, message []byte) error {
	_, err := r.cli.Publish(ctx, &pb.RedisPubRequest{Channel: channel, Message: message})
	return err
}

// Subscribe opens a server-streaming RPC and forwards every received message
// to the returned channel. The channel is closed when the stream ends or
// when ctx is cancelled.
func (r *RedisClient) Subscribe(ctx context.Context, channels ...string) (<-chan RedisMsg, error) {
	if len(channels) == 0 {
		return nil, fmt.Errorf("subscribe requires at least one channel")
	}
	stream, err := r.cli.Subscribe(ctx, &pb.RedisSubRequest{Channels: channels})
	if err != nil {
		return nil, err
	}
	out := make(chan RedisMsg, 16)
	go func() {
		defer close(out)
		for {
			msg, recvErr := stream.Recv()
			if recvErr != nil {
				if !errors.Is(recvErr, io.EOF) && ctx.Err() == nil {
					r.logger.Error("redis subscribe stream terminated", "error", recvErr)
				}
				return
			}
			select {
			case out <- RedisMsg{Channel: msg.GetChannel(), Data: msg.GetData()}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// anySlice converts []string to []any. Generic helper used by command builders
// that accept Redis arguments of varying shapes.
func anySlice(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
