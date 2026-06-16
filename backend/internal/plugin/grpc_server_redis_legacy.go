// grpc_server_redis_legacy.go isolates the legacy typed RedisProxy RPCs
// (Get / Set / SetEx / Del / HGet / HSet / HGetAll / HDel / Publish /
// Subscribe). Before the file split they lived in grpc_server.go and pushed
// that file past 675 lines.
//
// 这些方法保留是为了兼容旧 SDK 二进制。**新代码应通过 Do() 调用** (见
// grpc_server_redis_do.go),由 Do() 负责命名空间、能力检查等安全控制.
// 当所有插件都升级到使用 Do() 的 SDK 后,这些方法可以删除。
//
// 这里的 legacy 方法把 key 当作原始字符串透传给 Redis,不做命名空间校验,
// 因为旧 SDK 客户端自己手动拼接 `plugin:<name>:` 前缀(见 channel-management
// 的 cache_writer.go 和 hello-world 的旧版 main.go)。如果在这里再加一层
// 前缀会出现 `plugin:foo:plugin:foo:bar` 的双重前缀。
//
// 关于 caller 鉴权:RequirePluginIdentityUnary/Stream 拦截器已在 ctx 上塞好
// 已校验的 plugin name,所以这里的 handler 无需再调用 resolveCaller —— 直接
// CallerFromContext 即可,空名永远不会进入(拦截器先行 PermissionDenied)。

package plugin

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

func (s *SDKServer) Get(ctx context.Context, req *pluginsdk.RedisKeyRequest) (*pluginsdk.RedisValueResponse, error) {
	if s.redis == nil {
		return nil, errService("redis")
	}
	val, err := s.redis.Get(ctx, req.GetKey()).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return &pluginsdk.RedisValueResponse{Exists: false}, nil
		}
		return nil, errInternal(err, "plugin redis get")
	}
	return &pluginsdk.RedisValueResponse{Exists: true, Value: val}, nil
}

func (s *SDKServer) Set(ctx context.Context, req *pluginsdk.RedisSetRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, errService("redis")
	}
	if err := s.redis.Set(ctx, req.GetKey(), req.GetValue(), 0).Err(); err != nil {
		return nil, errInternal(err, "plugin redis set")
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) SetEx(ctx context.Context, req *pluginsdk.RedisSetExRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, errService("redis")
	}
	ttl := time.Duration(req.GetTtlSeconds()) * time.Second
	if err := s.redis.Set(ctx, req.GetKey(), req.GetValue(), ttl).Err(); err != nil {
		return nil, errInternal(err, "plugin redis setex")
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) Del(ctx context.Context, req *pluginsdk.RedisDelRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, errService("redis")
	}
	keys := req.GetKeys()
	if len(keys) == 0 {
		return &emptypb.Empty{}, nil
	}
	if err := s.redis.Del(ctx, keys...).Err(); err != nil {
		return nil, errInternal(err, "plugin redis del")
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) HGet(ctx context.Context, req *pluginsdk.RedisHGetRequest) (*pluginsdk.RedisValueResponse, error) {
	if s.redis == nil {
		return nil, errService("redis")
	}
	val, err := s.redis.HGet(ctx, req.GetKey(), req.GetField()).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return &pluginsdk.RedisValueResponse{Exists: false}, nil
		}
		return nil, errInternal(err, "plugin redis hget")
	}
	return &pluginsdk.RedisValueResponse{Exists: true, Value: val}, nil
}

func (s *SDKServer) HSet(ctx context.Context, req *pluginsdk.RedisHSetRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, errService("redis")
	}
	if err := s.redis.HSet(ctx, req.GetKey(), req.GetField(), req.GetValue()).Err(); err != nil {
		return nil, errInternal(err, "plugin redis hset")
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) HGetAll(ctx context.Context, req *pluginsdk.RedisKeyRequest) (*pluginsdk.RedisMapResponse, error) {
	if s.redis == nil {
		return nil, errService("redis")
	}
	m, err := s.redis.HGetAll(ctx, req.GetKey()).Result()
	if err != nil {
		return nil, errInternal(err, "plugin redis hgetall")
	}
	out := &pluginsdk.RedisMapResponse{Fields: make(map[string][]byte, len(m))}
	for k, v := range m {
		out.Fields[k] = []byte(v)
	}
	return out, nil
}

func (s *SDKServer) HDel(ctx context.Context, req *pluginsdk.RedisHDelRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, errService("redis")
	}
	if err := s.redis.HDel(ctx, req.GetKey(), req.GetFields()...).Err(); err != nil {
		return nil, errInternal(err, "plugin redis hdel")
	}
	return &emptypb.Empty{}, nil
}

func (s *SDKServer) Publish(ctx context.Context, req *pluginsdk.RedisPubRequest) (*emptypb.Empty, error) {
	if s.redis == nil {
		return nil, errService("redis")
	}
	if err := s.redis.Publish(ctx, req.GetChannel(), req.GetMessage()).Err(); err != nil {
		return nil, errInternal(err, "plugin redis publish")
	}
	return &emptypb.Empty{}, nil
}

// Subscribe 订阅指定 channel,转发到 gRPC stream。
// 注意:此方法依赖 Redis pub/sub,与 EventBus 实现共用同一个 Redis 实例。
// 转发循环抽象在 forwardPubsubToStream 中 (T40),与 EventBus.Subscribe 共享。
func (s *SDKServer) Subscribe(req *pluginsdk.RedisSubRequest, stream grpc.ServerStreamingServer[pluginsdk.RedisMessage]) error {
	if s.redis == nil {
		return errService("redis")
	}
	channels := req.GetChannels()
	if len(channels) == 0 {
		return status.Error(codes.InvalidArgument, "at least one channel is required")
	}

	pubsub := s.redis.Subscribe(stream.Context(), channels...)
	defer func() { _ = pubsub.Close() }()

	return forwardPubsubToStream(stream.Context(), pubsub.Channel(), stream,
		func(msg *redis.Message) *pluginsdk.RedisMessage {
			return &pluginsdk.RedisMessage{
				Channel: msg.Channel,
				Data:    []byte(msg.Payload),
			}
		})
}
