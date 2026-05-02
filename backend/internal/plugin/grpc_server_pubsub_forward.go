// grpc_server_pubsub_forward.go centralises the loop that bridges go-redis
// pub/sub Channel into a gRPC server stream. Two SDK services need it —
// RedisProxy.Subscribe (legacy raw-channel forwarding) and EventBus.Subscribe
// (event-bus topics over the same Redis instance) — and before T40 they
// shipped a hand-rolled copy each. The for-select / chMap mapping was
// identical except for the message wrapper; that is exactly what generics
// were designed for.
//
// The helper assumes the caller already created the *redis.PubSub via
// redis.Subscribe and owns the close lifecycle (defer pubsub.Close()). We do
// not crack open the message envelope here either — encodeMsg is responsible
// for translating each *redis.Message into the wire type and (optionally)
// short-circuiting (return nil pointer) when the channel name is unknown.

package plugin

import (
	"context"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

// forwardPubsubToStream pumps messages from src onto stream until either ctx
// cancels (host disconnect) or the channel is closed by the broker. Each
// inbound message is converted via encodeMsg; nil return values are skipped
// so encoders can drop messages that fail their own filter (e.g. an event-bus
// topic that was unsubscribed mid-flight).
//
// Generic over T (the gRPC message type) so the same loop serves both
// RedisProxy and EventBus without forcing a shared interface on the proto
// types.
func forwardPubsubToStream[T any](
	ctx context.Context,
	src <-chan *redis.Message,
	stream grpc.ServerStream,
	encodeMsg func(*redis.Message) *T,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-src:
			if !ok {
				// pubsub closed by the broker — graceful EOF; the host will
				// reconnect on its own backoff schedule.
				return nil
			}
			out := encodeMsg(msg)
			if out == nil {
				continue
			}
			if err := stream.SendMsg(out); err != nil {
				return err
			}
		}
	}
}
