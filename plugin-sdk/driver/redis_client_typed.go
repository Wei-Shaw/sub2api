// redis_client_typed.go houses the typed go-redis-style convenience
// wrappers around RedisClient.Do. They are mechanical (build a
// DoRequest, dispatch through Do, parse the reply with one of the
// As* helpers) so keeping them in their own file lets the core
// dispatcher (redis_client.go) stay scannable without scrolling past
// 90 typed methods (T46 split).

package driver

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// ============================================================
// Cmdable — string commands
// ============================================================

// Get is the canonical "fetch a string by key" command. Missing key surfaces
// as ErrRedisNil so callers can branch with errors.Is.
func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	cmd := r.Do(ctx, "GET", []int{0}, key).AsString()
	return cmd.val, cmd.err
}

// Set writes value with no expiration. Mirrors go-redis's Cmdable.Set when
// expiration is 0.
func (r *RedisClient) Set(ctx context.Context, key string, value string) error {
	return r.Do(ctx, "SET", []int{0}, key, value).AsStatus().Err()
}

// SetEx writes value with the supplied TTL. ttl≤0 is treated as no
// expiration so the call collapses to SET, matching the original behaviour.
// Sub-second TTLs are rounded up to 1 second to keep things deterministic.
func (r *RedisClient) SetEx(ctx context.Context, key string, value string, ttl time.Duration) error {
	if ttl <= 0 {
		return r.Set(ctx, key, value)
	}
	seconds := int64(ttl.Seconds())
	if seconds <= 0 {
		seconds = 1
	}
	return r.Do(ctx, "SETEX", []int{0}, key, seconds, value).AsStatus().Err()
}

// SetNX is SET if-not-exists. Returns true when the key was set.
func (r *RedisClient) SetNX(ctx context.Context, key string, value string, ttl time.Duration) *BoolCmd {
	args := []any{key, value, "NX"}
	if ttl > 0 {
		args = append(args, "EX", int64(ttl.Seconds()))
	}
	return r.Do(ctx, "SET", []int{0}, args...).AsBool()
}

// GetSet writes value, returning the old value (or ErrRedisNil if absent).
func (r *RedisClient) GetSet(ctx context.Context, key, value string) *StringCmd {
	return r.Do(ctx, "GETSET", []int{0}, key, value).AsString()
}

// Incr increments the integer value at key.
func (r *RedisClient) Incr(ctx context.Context, key string) *IntCmd {
	return r.Do(ctx, "INCR", []int{0}, key).AsInt()
}

// IncrBy increments by the supplied delta.
func (r *RedisClient) IncrBy(ctx context.Context, key string, value int64) *IntCmd {
	return r.Do(ctx, "INCRBY", []int{0}, key, value).AsInt()
}

// Decr decrements the integer value at key.
func (r *RedisClient) Decr(ctx context.Context, key string) *IntCmd {
	return r.Do(ctx, "DECR", []int{0}, key).AsInt()
}

// DecrBy decrements by the supplied delta.
func (r *RedisClient) DecrBy(ctx context.Context, key string, value int64) *IntCmd {
	return r.Do(ctx, "DECRBY", []int{0}, key, value).AsInt()
}

// IncrByFloat increments by a float delta and returns the resulting value.
func (r *RedisClient) IncrByFloat(ctx context.Context, key string, value float64) *FloatCmd {
	return r.Do(ctx, "INCRBYFLOAT", []int{0}, key, value).AsFloat()
}

// Append appends value to the string at key.
func (r *RedisClient) Append(ctx context.Context, key, value string) *IntCmd {
	return r.Do(ctx, "APPEND", []int{0}, key, value).AsInt()
}

// StrLen returns the length of the string at key.
func (r *RedisClient) StrLen(ctx context.Context, key string) *IntCmd {
	return r.Do(ctx, "STRLEN", []int{0}, key).AsInt()
}

// MGet fetches multiple keys in one round trip.
func (r *RedisClient) MGet(ctx context.Context, keys ...string) *SliceCmd {
	args := make([]any, len(keys))
	positions := make([]int, len(keys))
	for i, k := range keys {
		args[i] = k
		positions[i] = i
	}
	return r.Do(ctx, "MGET", positions, args...).AsSlice()
}

// MSet writes multiple key/value pairs atomically. pairs must alternate
// key, value, key, value … as go-redis does.
func (r *RedisClient) MSet(ctx context.Context, pairs ...any) *StatusCmd {
	if len(pairs)%2 != 0 {
		cmd := &StatusCmd{}
		cmd.setErr(errors.New("redis: MSet requires key/value pairs"))
		return cmd
	}
	positions := make([]int, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		positions = append(positions, i)
	}
	return r.Do(ctx, "MSET", positions, pairs...).AsStatus()
}

// ============================================================
// Cmdable — generic / key-space commands
// ============================================================

// Del removes one or more keys.
func (r *RedisClient) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	args := make([]any, len(keys))
	positions := make([]int, len(keys))
	for i, k := range keys {
		args[i] = k
		positions[i] = i
	}
	return r.Do(ctx, "DEL", positions, args...).AsInt().Err()
}

// Exists returns the number of supplied keys that exist.
func (r *RedisClient) Exists(ctx context.Context, keys ...string) *IntCmd {
	args := make([]any, len(keys))
	positions := make([]int, len(keys))
	for i, k := range keys {
		args[i] = k
		positions[i] = i
	}
	return r.Do(ctx, "EXISTS", positions, args...).AsInt()
}

// Expire sets a TTL on key. Returns whether the timeout was applied.
func (r *RedisClient) Expire(ctx context.Context, key string, ttl time.Duration) *BoolCmd {
	return r.Do(ctx, "EXPIRE", []int{0}, key, int64(ttl.Seconds())).AsBool()
}

// ExpireAt sets the absolute expiry timestamp.
func (r *RedisClient) ExpireAt(ctx context.Context, key string, t time.Time) *BoolCmd {
	return r.Do(ctx, "EXPIREAT", []int{0}, key, t.Unix()).AsBool()
}

// PExpire sets a TTL with millisecond precision.
func (r *RedisClient) PExpire(ctx context.Context, key string, ttl time.Duration) *BoolCmd {
	return r.Do(ctx, "PEXPIRE", []int{0}, key, ttl.Milliseconds()).AsBool()
}

// TTL returns the remaining TTL in seconds.
func (r *RedisClient) TTL(ctx context.Context, key string) *DurationCmd {
	cmd := r.Do(ctx, "TTL", []int{0}, key)
	return parseDurationCmd(cmd.reply, cmd.err, time.Second)
}

// PTTL returns the remaining TTL in milliseconds.
func (r *RedisClient) PTTL(ctx context.Context, key string) *DurationCmd {
	cmd := r.Do(ctx, "PTTL", []int{0}, key)
	return parseDurationCmd(cmd.reply, cmd.err, time.Millisecond)
}

// Type returns the type of the value stored at key.
func (r *RedisClient) Type(ctx context.Context, key string) *StatusCmd {
	return r.Do(ctx, "TYPE", []int{0}, key).AsStatus()
}

// Rename renames key to newkey. Both keys are namespaced.
func (r *RedisClient) Rename(ctx context.Context, key, newkey string) *StatusCmd {
	return r.Do(ctx, "RENAME", []int{0, 1}, key, newkey).AsStatus()
}

// Persist removes the TTL from key.
func (r *RedisClient) Persist(ctx context.Context, key string) *BoolCmd {
	return r.Do(ctx, "PERSIST", []int{0}, key).AsBool()
}

// Keys returns keys matching pattern. The pattern is itself namespaced so a
// plugin only ever sees its own keys when calling KEYS *.
func (r *RedisClient) Keys(ctx context.Context, pattern string) *StringSliceCmd {
	// pattern always lives in arg position 0 (it is the only argument), and
	// it is treated as a key-space pattern so we apply the prefix.
	return r.Do(ctx, "KEYS", []int{0}, pattern).AsStringSlice()
}

// Scan paginates the key space. The MATCH pattern is namespaced.
func (r *RedisClient) Scan(ctx context.Context, cursor uint64, match string, count int64) *ScanCmd {
	args := []any{cursor}
	keyPositions := []int{}
	if match != "" {
		args = append(args, "MATCH", match)
		// MATCH pattern is at index 2 in args, but key-positions are
		// indices in args excluding the command, which is also 2.
		keyPositions = append(keyPositions, 2)
	}
	if count > 0 {
		args = append(args, "COUNT", count)
	}
	cmd := r.Do(ctx, "SCAN", keyPositions, args...)
	return parseScanCmd(cmd.reply, cmd.err, r.namespace)
}

// ScanCmd is the result of SCAN/HSCAN/SSCAN/ZSCAN: cursor + matched keys.
type ScanCmd struct {
	baseCmd
	cursor uint64
	keys   []string
}

func (c *ScanCmd) Val() (uint64, []string)           { return c.cursor, c.keys }
func (c *ScanCmd) Result() (uint64, []string, error) { return c.cursor, c.keys, c.err }

// parseScanCmd handles the [cursor, [keys...]] reply shape and strips the
// per-plugin namespace from each returned key so the caller does not have to.
func parseScanCmd(reply *pb.DoReply, err error, namespace string) *ScanCmd {
	cmd := &ScanCmd{}
	if applyTransportErr(cmd, err) || applyReplyErr(cmd, reply) {
		return cmd
	}
	if reply.GetKind() != replyKindArray || len(reply.GetArray()) != 2 {
		cmd.setErr(errUnexpectedReply(reply.GetKind()))
		return cmd
	}
	cursorEl := reply.GetArray()[0]
	cursorStr := replyAsString(cursorEl)
	if cursorStr != "" {
		c, perr := strconv.ParseUint(cursorStr, 10, 64)
		if perr != nil {
			cmd.setErr(perr)
			return cmd
		}
		cmd.cursor = c
	}
	listEl := reply.GetArray()[1]
	if listEl.GetKind() == replyKindArray {
		out := make([]string, 0, len(listEl.GetArray()))
		for _, e := range listEl.GetArray() {
			k := replyAsString(e)
			if namespace != "" {
				k = strings.TrimPrefix(k, namespace)
			}
			out = append(out, k)
		}
		cmd.keys = out
	}
	return cmd
}
