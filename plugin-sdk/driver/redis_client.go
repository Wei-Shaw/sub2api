// Package driver contains the gRPC-backed implementations of the SDK's
// SQL driver and Redis client. They are kept in a sub-package so plugins do
// not see the proto types unless they really need them.
package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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

// RedisClient is the gRPC-backed implementation of pluginsdk.RedisClient.
type RedisClient struct {
	cli    pb.RedisProxyClient
	logger *slog.Logger
}

// NewRedisClient builds a RedisClient that talks to the supplied
// RedisProxyClient. The logger is used for asynchronous errors (subscribe
// stream failures) that cannot be returned to the caller directly.
func NewRedisClient(cli pb.RedisProxyClient, logger *slog.Logger) *RedisClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &RedisClient{cli: cli, logger: logger}
}

// Get returns the value of key. If the key does not exist the call returns
// "" together with ErrRedisNil so callers can distinguish absence from empty.
func (r *RedisClient) Get(ctx context.Context, key string) (string, error) {
	resp, err := r.cli.Get(ctx, &pb.RedisKeyRequest{Key: key})
	if err != nil {
		return "", err
	}
	if !resp.GetExists() {
		return "", ErrRedisNil
	}
	return string(resp.GetValue()), nil
}

// Set writes value at key with no expiration.
func (r *RedisClient) Set(ctx context.Context, key string, value string) error {
	_, err := r.cli.Set(ctx, &pb.RedisSetRequest{Key: key, Value: []byte(value)})
	return err
}

// SetEx writes value at key with the given TTL. A non-positive ttl is treated
// as no expiration to match Redis semantics.
func (r *RedisClient) SetEx(ctx context.Context, key string, value string, ttl time.Duration) error {
	seconds := int64(ttl.Seconds())
	if seconds <= 0 && ttl > 0 {
		// ttl smaller than one second still rounds up so the key is not
		// kept forever.
		seconds = 1
	}
	_, err := r.cli.SetEx(ctx, &pb.RedisSetExRequest{
		Key:        key,
		Value:      []byte(value),
		TtlSeconds: seconds,
	})
	return err
}

// Del deletes one or more keys. Deleting zero keys is a no-op.
func (r *RedisClient) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := r.cli.Del(ctx, &pb.RedisDelRequest{Keys: keys})
	return err
}

// HGet reads a single field from a hash. Missing field returns ErrRedisNil.
func (r *RedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	resp, err := r.cli.HGet(ctx, &pb.RedisHGetRequest{Key: key, Field: field})
	if err != nil {
		return "", err
	}
	if !resp.GetExists() {
		return "", ErrRedisNil
	}
	return string(resp.GetValue()), nil
}

// HSet writes a single field on a hash.
func (r *RedisClient) HSet(ctx context.Context, key, field, value string) error {
	_, err := r.cli.HSet(ctx, &pb.RedisHSetRequest{
		Key:   key,
		Field: field,
		Value: []byte(value),
	})
	return err
}

// HGetAll returns all fields of a hash. An empty hash returns an empty map
// (not nil) so callers can range without nil checks.
func (r *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	resp, err := r.cli.HGetAll(ctx, &pb.RedisKeyRequest{Key: key})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(resp.GetFields()))
	for k, v := range resp.GetFields() {
		out[k] = string(v)
	}
	return out, nil
}

// HDel deletes the named fields from a hash.
func (r *RedisClient) HDel(ctx context.Context, key string, fields ...string) error {
	if len(fields) == 0 {
		return nil
	}
	_, err := r.cli.HDel(ctx, &pb.RedisHDelRequest{Key: key, Fields: fields})
	return err
}

// Publish broadcasts a message on the given channel.
func (r *RedisClient) Publish(ctx context.Context, channel string, message []byte) error {
	_, err := r.cli.Publish(ctx, &pb.RedisPubRequest{Channel: channel, Message: message})
	return err
}

// Subscribe opens a server-streaming RPC and forwards every received message
// to the returned channel. The channel is closed when the stream ends or
// when ctx is cancelled. Stream errors other than io.EOF / context
// cancellation are logged but not returned, since the channel is the only
// signal callers ever observe after a successful Subscribe.
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
