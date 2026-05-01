// Package driver contains the gRPC-backed implementations of the SDK's
// SQL driver and Redis client. They are kept in a sub-package so plugins do
// not see the proto types unless they really need them.
//
// Redis client design
// -------------------
// The client mirrors github.com/redis/go-redis/v9's Cmdable surface so plugin
// code reads identically to native go-redis. Internally every method
// constructs a DoRequest and dispatches it through the RedisProxy.Do RPC.
// The legacy typed RPCs (Get/Set/HSet/...) are no longer used here — they
// remain in the proto file purely so older plugin binaries keep working
// against a newer core.
//
// Per-plugin namespacing is implemented in two layers:
//
//   1. Client side (this file) attaches the prefix `plugin:<name>:` to the
//      key arguments of every command before serialising the DoRequest.
//      The client has the full picture of which arguments are keys; the
//      core does not.
//   2. Server side (backend/internal/plugin/grpc_server.go) verifies the
//      prefix as defence in depth so a misbehaving SDK can never write
//      outside its sandbox.
//
// Plugins that need to operate on shared keys (e.g. cross-plugin caches)
// can opt out by declaring the `redis_raw_keys` capability in their manifest
// and using `client.Raw()` to obtain a non-namespaced client.

package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// ErrRedisNil mirrors go-redis's redis.Nil sentinel for missing keys. Code
// that switches on errors.Is(err, ErrRedisNil) gets behaviour familiar to
// existing Redis users.
var ErrRedisNil = errors.New("redis: nil")

// RedisMsg is the SDK pub/sub message. Re-declared here so the driver package
// has no import cycle with the parent pluginsdk package.
type RedisMsg struct {
	Channel string
	Data    []byte
}

// RedisClient is the gRPC-backed implementation of the SDK's RedisClient
// interface. It exposes a go-redis Cmdable-style surface and routes every
// command through RedisProxy.Do.
type RedisClient struct {
	cli        pb.RedisProxyClient
	logger     *slog.Logger
	namespace  string       // "" = no prefixing (Raw mode); else "plugin:<name>:"
	rawAllowed bool         // true if the plugin declared redis_raw_keys
	rawClient  *RedisClient // lazily-initialised raw twin (created by Raw())
}

// NewRedisClient builds a RedisClient with the default per-plugin namespace
// disabled. Use NewNamespacedRedisClient for plugins that want isolation.
//
// Kept as the legacy entry point so older runners that called this directly
// keep compiling. New code should always go through NewNamespacedRedisClient.
func NewRedisClient(cli pb.RedisProxyClient, logger *slog.Logger) *RedisClient {
	return NewNamespacedRedisClient(cli, logger, "", false)
}

// NewNamespacedRedisClient builds a RedisClient that prefixes every key with
// `plugin:<pluginName>:`. If pluginName is empty the client behaves like a
// raw client (no prefixing). rawAllowed controls whether Raw() returns a
// usable raw twin or nil.
func NewNamespacedRedisClient(cli pb.RedisProxyClient, logger *slog.Logger, pluginName string, rawAllowed bool) *RedisClient {
	if logger == nil {
		logger = slog.Default()
	}
	ns := ""
	if pluginName != "" {
		ns = "plugin:" + pluginName + ":"
	}
	return &RedisClient{
		cli:        cli,
		logger:     logger,
		namespace:  ns,
		rawAllowed: rawAllowed,
	}
}

// Raw returns a sibling client that bypasses the per-plugin namespace.
// Returns nil for plugins that have not declared the `redis_raw_keys`
// capability — callers should treat nil as "feature disabled" and surface
// a clear error to their users.
//
// The same gRPC connection is reused; only the local prefixing logic is
// disabled. The server still validates raw_key=true against the plugin's
// approved capabilities so a hijacked SDK cannot grant itself raw access.
func (r *RedisClient) Raw() *RedisClient {
	if !r.rawAllowed {
		return nil
	}
	if r.rawClient != nil {
		return r.rawClient
	}
	r.rawClient = &RedisClient{
		cli:        r.cli,
		logger:     r.logger,
		namespace:  "",
		rawAllowed: r.rawAllowed,
	}
	return r.rawClient
}

// Namespace returns the active key prefix ("" when running in raw mode).
// Mostly useful for logging and tests.
func (r *RedisClient) Namespace() string { return r.namespace }

// applyKey adds the namespace prefix unless the client is in raw mode.
//
// 暂未在 SDK 现版本里被任何 typed Cmdable 包装直接调用 —— 当前所有
// 命名空间前缀都在 host 侧 RedisProxy 完成。保留此方法是因为后续若把
// 命名空间应用上移至 SDK（例如批量 MGET 优化），是显然的入口。
//
//nolint:unused // SDK helper kept for future namespace handling, see comment above.
func (r *RedisClient) applyKey(key string) string {
	if r.namespace == "" || strings.HasPrefix(key, r.namespace) {
		return key
	}
	return r.namespace + key
}

// applyKeys batch-prefixes keys.
//
//nolint:unused // SDK helper kept for future namespace handling; pairs with applyKey.
func (r *RedisClient) applyKeys(keys []string) []string {
	if r.namespace == "" {
		return keys
	}
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = r.applyKey(k)
	}
	return out
}

// ============================================================
// Do — universal entry point
// ============================================================

// Do issues an arbitrary Redis command. Plugin authors who want full control
// (or commands the typed Cmdable methods do not cover) call this directly.
//
// keyPositions are indices into args (0-based, into the args slice excluding
// the command name) that contain Redis keys. Supplying it lets the client
// apply the namespace prefix correctly. Pass nil to mean "no key arguments"
// (e.g. PING, INFO).
//
// The returned *DoCmd lets the caller inspect the raw reply or call one of
// the typed As* helpers.
func (r *RedisClient) Do(ctx context.Context, command string, keyPositions []int, args ...any) *DoCmd {
	cmd := &DoCmd{}
	encoded, err := encodeArgs(command, args)
	if err != nil {
		cmd.setErr(err)
		return cmd
	}
	// Apply namespace to key positions. keyPositions is into args (excluding
	// the command), so add 1 to align with the encoded slice.
	if r.namespace != "" {
		for _, pos := range keyPositions {
			argIdx := pos + 1
			if argIdx < 0 || argIdx >= len(encoded) {
				continue
			}
			encoded[argIdx] = []byte(r.namespace + string(encoded[argIdx]))
		}
	}
	rawKey := r.namespace == "" && r.rawAllowed
	pbPos := make([]int32, 0, len(keyPositions))
	for _, p := range keyPositions {
		pbPos = append(pbPos, int32(p))
	}
	reply, err := r.cli.Do(ctx, &pb.DoRequest{
		Args:         encoded,
		RawKey:       rawKey,
		KeyPositions: pbPos,
	})
	if err != nil {
		cmd.setErr(err)
		return cmd
	}
	cmd.reply = reply
	if reply.GetKind() == replyKindError && reply.GetError() != "" {
		cmd.setErr(errors.New(reply.GetError()))
	}
	return cmd
}

// DoCmd is the result of a raw Do call. It exposes the raw DoReply plus
// helpers that mirror the typed Cmd parsers.
type DoCmd struct {
	baseCmd
	reply *pb.DoReply
}

// Reply returns the raw protobuf reply. May be nil on transport errors.
func (c *DoCmd) Reply() *pb.DoReply { return c.reply }

// AsString interprets the reply as a *StringCmd-like result.
func (c *DoCmd) AsString() *StringCmd { return parseStringCmd(c.reply, c.err) }

// AsInt interprets the reply as an *IntCmd-like result.
func (c *DoCmd) AsInt() *IntCmd { return parseIntCmd(c.reply, c.err) }

// AsBool interprets the reply as a *BoolCmd-like result.
func (c *DoCmd) AsBool() *BoolCmd { return parseBoolCmd(c.reply, c.err) }

// AsStringSlice interprets the reply as a *StringSliceCmd-like result.
func (c *DoCmd) AsStringSlice() *StringSliceCmd { return parseStringSliceCmd(c.reply, c.err) }

// AsStringStringMap interprets the reply as a *StringStringMapCmd result.
func (c *DoCmd) AsStringStringMap() *StringStringMapCmd {
	return parseStringStringMapCmd(c.reply, c.err)
}

// AsSlice interprets the reply as an untyped []any.
func (c *DoCmd) AsSlice() *SliceCmd { return parseSliceCmd(c.reply, c.err) }

// AsStatus interprets the reply as a *StatusCmd.
func (c *DoCmd) AsStatus() *StatusCmd { return parseStatusCmd(c.reply, c.err) }

// AsFloat interprets the reply as a *FloatCmd.
func (c *DoCmd) AsFloat() *FloatCmd { return parseFloatCmd(c.reply, c.err) }

// encodeArgs serialises (command, args...) into [][]byte. We accept the same
// argument types go-redis does so plugin code can move between the two with
// minimal changes.
func encodeArgs(command string, args []any) ([][]byte, error) {
	out := make([][]byte, 0, len(args)+1)
	out = append(out, []byte(command))
	for _, a := range args {
		b, err := encodeArg(a)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func encodeArg(a any) ([]byte, error) {
	switch v := a.(type) {
	case nil:
		return nil, errors.New("redis: nil argument")
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	case int:
		return []byte(strconv.FormatInt(int64(v), 10)), nil
	case int32:
		return []byte(strconv.FormatInt(int64(v), 10)), nil
	case int64:
		return []byte(strconv.FormatInt(v, 10)), nil
	case uint:
		return []byte(strconv.FormatUint(uint64(v), 10)), nil
	case uint32:
		return []byte(strconv.FormatUint(uint64(v), 10)), nil
	case uint64:
		return []byte(strconv.FormatUint(v, 10)), nil
	case float32:
		return []byte(strconv.FormatFloat(float64(v), 'f', -1, 32)), nil
	case float64:
		return []byte(strconv.FormatFloat(v, 'f', -1, 64)), nil
	case bool:
		if v {
			return []byte{'1'}, nil
		}
		return []byte{'0'}, nil
	case time.Duration:
		return []byte(strconv.FormatInt(int64(v/time.Second), 10)), nil
	case time.Time:
		return []byte(strconv.FormatInt(v.Unix(), 10)), nil
	case fmt.Stringer:
		return []byte(v.String()), nil
	default:
		return nil, fmt.Errorf("redis: unsupported argument type %T", a)
	}
}

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
