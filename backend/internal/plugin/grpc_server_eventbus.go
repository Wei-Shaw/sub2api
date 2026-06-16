// grpc_server_eventbus.go isolates the EventBus adapter that piggy-backs on
// the SDKServer's Redis client. EventBus.Publish/Subscribe live in their own
// file because their gRPC method names collide with RedisProxy's
// Publish/Subscribe at the receiver level — Go allows the collision because
// the EventBus methods hang off a separate eventBusAdapter type, but keeping
// them next to the legacy RedisProxy methods (grpc_server_redis_legacy.go)
// would obscure the boundary.
//
// The Subscribe forwarding loop reuses forwardPubsubToStream (T40) so the
// "go-redis Channel -> gRPC server stream" plumbing stays single-sourced
// with RedisProxy.Subscribe.

package plugin

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

// eventBusChannelPrefix 是事件总线在 Redis 上的 channel 前缀,
// 避免与插件直接使用的 RedisProxy.Publish channel 命名冲突。
const eventBusChannelPrefix = "plugin_eventbus:"

// eventBusAdapter 把 EventBus.Publish/Subscribe 适配到 SDKServer。
// 单独的类型避免与 RedisProxy.Publish(*RedisPubRequest) 形成方法签名冲突。
type eventBusAdapter struct {
	pluginsdk.UnimplementedEventBusServer
	srv *SDKServer
}

func (a *eventBusAdapter) Publish(ctx context.Context, req *pluginsdk.EventRequest) (*emptypb.Empty, error) {
	if a.srv.redis == nil {
		return nil, errService("redis")
	}
	channel := eventBusChannelPrefix + req.GetTopic()
	if err := a.srv.redis.Publish(ctx, channel, req.GetPayload()).Err(); err != nil {
		return nil, errInternal(err, "plugin eventbus publish")
	}
	return &emptypb.Empty{}, nil
}

func (a *eventBusAdapter) Subscribe(req *pluginsdk.EventFilter, stream grpc.ServerStreamingServer[pluginsdk.Event]) error {
	return a.srv.eventBusSubscribe(req, stream)
}

// eventBusSubscribe lives on SDKServer (rather than on the adapter) so the
// adapter stays a thin shim around the SDKServer's Redis client. The body
// uses forwardPubsubToStream (T40) for the actual loop.
func (s *SDKServer) eventBusSubscribe(req *pluginsdk.EventFilter, stream grpc.ServerStreamingServer[pluginsdk.Event]) error {
	if s.redis == nil {
		return errService("redis")
	}
	topics := req.GetTopics()
	if len(topics) == 0 {
		return status.Error(codes.InvalidArgument, "at least one topic is required")
	}
	channels := make([]string, 0, len(topics))
	chMap := make(map[string]string, len(topics)) // channel -> topic
	for _, t := range topics {
		ch := eventBusChannelPrefix + t
		channels = append(channels, ch)
		chMap[ch] = t
	}

	pubsub := s.redis.Subscribe(stream.Context(), channels...)
	defer func() { _ = pubsub.Close() }()

	return forwardPubsubToStream(stream.Context(), pubsub.Channel(), stream,
		func(msg *redis.Message) *pluginsdk.Event {
			topic := chMap[msg.Channel]
			if topic == "" {
				topic = msg.Channel
			}
			return &pluginsdk.Event{
				Topic:     topic,
				Payload:   []byte(msg.Payload),
				Timestamp: time.Now().UnixMilli(),
			}
		})
}
