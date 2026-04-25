package pluginsdk

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
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
}

// RedisClient is the minimal subset of Redis operations the SDK exposes to
// plugins. It maps 1:1 onto the RedisProxy gRPC service.
type RedisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string) error
	SetEx(ctx context.Context, key string, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	HGet(ctx context.Context, key, field string) (string, error)
	HSet(ctx context.Context, key, field, value string) error
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HDel(ctx context.Context, key string, fields ...string) error
	Publish(ctx context.Context, channel string, message []byte) error

	// Subscribe opens a server-streaming RPC and returns a channel of
	// messages. The channel is closed when the underlying stream ends or
	// when ctx is cancelled. Errors that terminate the stream are logged by
	// the SDK; callers detect termination via the channel close.
	Subscribe(ctx context.Context, channels ...string) (<-chan RedisMsg, error)
}

// RedisMsg is the SDK-level representation of a pub/sub message.
type RedisMsg struct {
	Channel string
	Data    []byte
}
