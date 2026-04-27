package pluginsdk

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	sdkdriver "github.com/Wei-Shaw/sub2api/plugin-sdk/driver"
)

// PluginContext bundles the resources the SDK provides to a Plugin during
// Init. It is implemented by the SDK; plugins should not implement it
// themselves.
//
// All resources are owned by the SDK and freed when the plugin shuts down.
// Plugins must not call Close on the returned *sql.DB or RedisClient — doing
// so will tear down the gRPC channel before Shutdown is invoked.
type PluginContext interface {
	// DB returns a *sql.DB whose driver proxies all queries through the
	// core's gRPC SQLProxy. The returned handle is safe for concurrent use
	// and can be passed straight to ent.NewClient or other ORMs that expect
	// the standard database/sql interface.
	DB() *sql.DB

	// Redis returns a Redis client backed by the core's gRPC RedisProxy.
	Redis() RedisClient

	// Logger returns a slog.Logger pre-tagged with the plugin's name. Plugins
	// should use this rather than slog.Default to keep logs attributable.
	Logger() *slog.Logger

	// Config returns the plain-string configuration map the core supplied in
	// the Init request. The map is a copy; mutating it has no effect.
	Config() map[string]string

	// Secrets returns the SecretEncryptor backed by the host's
	// SecretEncryption gRPC service. Returns nil when the plugin did not
	// declare CapabilitySecretEncryption — the SDK refuses to wire the
	// client in that case so a forgotten manifest entry surfaces as an
	// obvious nil-pointer instead of silent passthrough.
	Secrets() SecretEncryptor
}

// RedisClient is the SDK's go-redis-style Redis client. It mirrors the
// portion of github.com/redis/go-redis/v9.Cmdable that plugin authors need
// in practice. Internally every method routes through the core's RedisProxy
// gRPC service.
//
// Key namespacing
//
// By default keys are silently prefixed with `plugin:<plugin_name>:` so a
// plugin cannot accidentally clobber another plugin's data. Plugins that
// must read/write shared keys (e.g. the gateway cache contract maintained
// by channel-management) declare the CapabilityRedisRawKeys capability in
// their manifest and call Raw() to obtain a non-namespacing twin.
//
// Cmder helpers
//
// Read-style methods that returned simple values in v0 (Get, HGet, …) keep
// their original (string, error) shape so existing plugins compile
// unchanged. New methods follow the go-redis cmder pattern: they return a
// typed *XxxCmd and the caller pulls the value out via .Result(), .Val(),
// or .Err(). The v0 ergonomics are preserved on the methods that already
// shipped in v0 to avoid an SDK-wide breaking change.
type RedisClient interface {
	// ----- Generic / key-space -----
	Del(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, keys ...string) *sdkdriver.IntCmd
	Expire(ctx context.Context, key string, ttl time.Duration) *sdkdriver.BoolCmd
	ExpireAt(ctx context.Context, key string, t time.Time) *sdkdriver.BoolCmd
	PExpire(ctx context.Context, key string, ttl time.Duration) *sdkdriver.BoolCmd
	TTL(ctx context.Context, key string) *sdkdriver.DurationCmd
	PTTL(ctx context.Context, key string) *sdkdriver.DurationCmd
	Type(ctx context.Context, key string) *sdkdriver.StatusCmd
	Rename(ctx context.Context, key, newkey string) *sdkdriver.StatusCmd
	Persist(ctx context.Context, key string) *sdkdriver.BoolCmd
	Keys(ctx context.Context, pattern string) *sdkdriver.StringSliceCmd
	Scan(ctx context.Context, cursor uint64, match string, count int64) *sdkdriver.ScanCmd

	// ----- String -----
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string) error
	SetEx(ctx context.Context, key string, value string, ttl time.Duration) error
	SetNX(ctx context.Context, key string, value string, ttl time.Duration) *sdkdriver.BoolCmd
	GetSet(ctx context.Context, key, value string) *sdkdriver.StringCmd
	Incr(ctx context.Context, key string) *sdkdriver.IntCmd
	IncrBy(ctx context.Context, key string, value int64) *sdkdriver.IntCmd
	Decr(ctx context.Context, key string) *sdkdriver.IntCmd
	DecrBy(ctx context.Context, key string, value int64) *sdkdriver.IntCmd
	IncrByFloat(ctx context.Context, key string, value float64) *sdkdriver.FloatCmd
	Append(ctx context.Context, key, value string) *sdkdriver.IntCmd
	StrLen(ctx context.Context, key string) *sdkdriver.IntCmd
	MGet(ctx context.Context, keys ...string) *sdkdriver.SliceCmd
	MSet(ctx context.Context, pairs ...any) *sdkdriver.StatusCmd

	// ----- Hash -----
	HGet(ctx context.Context, key, field string) (string, error)
	HSet(ctx context.Context, key, field, value string) error
	HDel(ctx context.Context, key string, fields ...string) error
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HExists(ctx context.Context, key, field string) *sdkdriver.BoolCmd
	HKeys(ctx context.Context, key string) *sdkdriver.StringSliceCmd
	HVals(ctx context.Context, key string) *sdkdriver.StringSliceCmd
	HLen(ctx context.Context, key string) *sdkdriver.IntCmd
	HIncrBy(ctx context.Context, key, field string, incr int64) *sdkdriver.IntCmd
	HIncrByFloat(ctx context.Context, key, field string, incr float64) *sdkdriver.FloatCmd
	HMGet(ctx context.Context, key string, fields ...string) *sdkdriver.SliceCmd
	HMSet(ctx context.Context, key string, fields map[string]any) *sdkdriver.StatusCmd

	// ----- List -----
	LPush(ctx context.Context, key string, values ...any) *sdkdriver.IntCmd
	RPush(ctx context.Context, key string, values ...any) *sdkdriver.IntCmd
	LPop(ctx context.Context, key string) *sdkdriver.StringCmd
	RPop(ctx context.Context, key string) *sdkdriver.StringCmd
	LRange(ctx context.Context, key string, start, stop int64) *sdkdriver.StringSliceCmd
	LLen(ctx context.Context, key string) *sdkdriver.IntCmd
	LIndex(ctx context.Context, key string, index int64) *sdkdriver.StringCmd
	LRem(ctx context.Context, key string, count int64, value any) *sdkdriver.IntCmd
	LTrim(ctx context.Context, key string, start, stop int64) *sdkdriver.StatusCmd

	// ----- Set -----
	SAdd(ctx context.Context, key string, members ...any) *sdkdriver.IntCmd
	SRem(ctx context.Context, key string, members ...any) *sdkdriver.IntCmd
	SMembers(ctx context.Context, key string) *sdkdriver.StringSliceCmd
	SIsMember(ctx context.Context, key string, member any) *sdkdriver.BoolCmd
	SCard(ctx context.Context, key string) *sdkdriver.IntCmd
	SPop(ctx context.Context, key string) *sdkdriver.StringCmd
	SRandMember(ctx context.Context, key string) *sdkdriver.StringCmd

	// ----- Sorted Set -----
	ZAdd(ctx context.Context, key string, members ...sdkdriver.ZAddArgs) *sdkdriver.IntCmd
	ZRem(ctx context.Context, key string, members ...any) *sdkdriver.IntCmd
	ZRange(ctx context.Context, key string, start, stop int64) *sdkdriver.StringSliceCmd
	ZRevRange(ctx context.Context, key string, start, stop int64) *sdkdriver.StringSliceCmd
	ZRangeByScore(ctx context.Context, key string, min, max string) *sdkdriver.StringSliceCmd
	ZScore(ctx context.Context, key, member string) *sdkdriver.FloatCmd
	ZIncrBy(ctx context.Context, key string, incr float64, member string) *sdkdriver.FloatCmd
	ZRank(ctx context.Context, key, member string) *sdkdriver.IntCmd
	ZCard(ctx context.Context, key string) *sdkdriver.IntCmd
	ZCount(ctx context.Context, key, min, max string) *sdkdriver.IntCmd

	// ----- Server -----
	Ping(ctx context.Context) *sdkdriver.StatusCmd

	// ----- Scripting -----
	Eval(ctx context.Context, script string, keys []string, args ...any) *sdkdriver.DoCmd
	EvalSha(ctx context.Context, sha1 string, keys []string, args ...any) *sdkdriver.DoCmd
	ScriptLoad(ctx context.Context, script string) *sdkdriver.StringCmd

	// ----- Pub/Sub -----
	Publish(ctx context.Context, channel string, message []byte) error
	Subscribe(ctx context.Context, channels ...string) (<-chan RedisMsg, error)

	// ----- Universal escape hatch -----
	// Do dispatches an arbitrary Redis command. keyPositions are the indices
	// (0-based, into the args slice excluding the command name) that contain
	// keys; the SDK applies the per-plugin namespace to those positions.
	// Pass nil for commands without key arguments (PING, INFO).
	Do(ctx context.Context, command string, keyPositions []int, args ...any) *sdkdriver.DoCmd

	// Raw returns a sibling client that bypasses the per-plugin namespace.
	// Returns nil when the plugin lacks the CapabilityRedisRawKeys capability.
	// Plugins that share keys with the core or other plugins (e.g. the
	// channel-management gateway cache contract) MUST use this.
	Raw() RedisClient
}

// RedisMsg is the SDK-level representation of a pub/sub message.
type RedisMsg struct {
	Channel string
	Data    []byte
}
