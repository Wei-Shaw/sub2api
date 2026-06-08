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
	"log/slog"
	"strconv"
	"strings"
	"sync"
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
	rawOnce    sync.Once    // guards lazy init of rawClient
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
	r.rawOnce.Do(func() {
		r.rawClient = &RedisClient{
			cli:        r.cli,
			logger:     r.logger,
			namespace:  "",
			rawAllowed: r.rawAllowed,
		}
	})
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
