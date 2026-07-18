package inbox

import (
	"context"
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/redis/go-redis/v9"
)

// Redis pub/sub 频道：发布节点广播通知，所有节点订阅后向本地在线连接投递，实现
// 跨节点实时 fan-out。消息只承载"通知"，不承载 ack 状态——每个节点按本地连接的
// 用户各自派生 unacked。
const (
	channelNotifyDirect    = "inbox:notify:direct"
	channelNotifyBroadcast = "inbox:notify:broadcast"
	// channelKick 承载"某用户新建连接"事件，用于跨实例单连接踢出。
	channelKick = "inbox:kick"
)

// directEnvelope 是单播通知在 pub/sub 上的载荷。
type directEnvelope struct {
	UserID  int64   `json:"user_id"`
	Message Message `json:"message"`
}

// kickEnvelope 是跨实例踢出事件的载荷：用户 uid 在某实例新建了连接 connID，
// 其它实例上该用户 connID 不同的旧连接应被踢出。
type kickEnvelope struct {
	UserID int64 `json:"user_id"`
	ConnID int64 `json:"conn_id"`
}

// Coordinator 实现 Notifier：把发布事件投递到 Redis pub/sub；同时订阅这些频道，将
// 事件分发到本节点的 Hub。单节点部署时同样工作（自己发自己收）。
type Coordinator struct {
	rdb *redis.Client
	hub *Hub
}

// NewCoordinator 构造协调器。
func NewCoordinator(rdb *redis.Client, hub *Hub) *Coordinator {
	return &Coordinator{rdb: rdb, hub: hub}
}

// NotifyDirect 实现 Notifier：发布单播通知。
func (c *Coordinator) NotifyDirect(ctx context.Context, userID int64, m Message) {
	if c == nil || c.rdb == nil {
		return
	}
	payload, err := json.Marshal(directEnvelope{UserID: userID, Message: m})
	if err != nil {
		logger.LegacyPrintf("inbox.coordinator", "[Inbox] marshal direct notify failed: %v", err)
		return
	}
	if err := c.rdb.Publish(ctx, channelNotifyDirect, payload).Err(); err != nil {
		logger.LegacyPrintf("inbox.coordinator", "[Inbox] publish direct notify failed: %v", err)
	}
}

// NotifyBroadcast 实现 Notifier：发布广播通知。
func (c *Coordinator) NotifyBroadcast(ctx context.Context, b Broadcast) {
	if c == nil || c.rdb == nil {
		return
	}
	payload, err := json.Marshal(b)
	if err != nil {
		logger.LegacyPrintf("inbox.coordinator", "[Inbox] marshal broadcast notify failed: %v", err)
		return
	}
	if err := c.rdb.Publish(ctx, channelNotifyBroadcast, payload).Err(); err != nil {
		logger.LegacyPrintf("inbox.coordinator", "[Inbox] publish broadcast notify failed: %v", err)
	}
}

// PublishKick 实现 KickBroadcaster：广播"用户 userID 新建了连接 connID"，供其它实例
// 踢出该用户的旧连接。
func (c *Coordinator) PublishKick(ctx context.Context, userID, connID int64) {
	if c == nil || c.rdb == nil {
		return
	}
	payload, err := json.Marshal(kickEnvelope{UserID: userID, ConnID: connID})
	if err != nil {
		logger.LegacyPrintf("inbox.coordinator", "[Inbox] marshal kick failed: %v", err)
		return
	}
	if err := c.rdb.Publish(ctx, channelKick, payload).Err(); err != nil {
		logger.LegacyPrintf("inbox.coordinator", "[Inbox] publish kick failed: %v", err)
	}
}

// Run 订阅 pub/sub 频道并把事件分发到本地 Hub，直到 ctx 取消。阻塞运行，应在独立
// goroutine 中启动。
func (c *Coordinator) Run(ctx context.Context) {
	if c == nil || c.rdb == nil || c.hub == nil {
		return
	}
	sub := c.rdb.Subscribe(ctx, channelNotifyDirect, channelNotifyBroadcast, channelKick)
	defer func() { _ = sub.Close() }()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			c.dispatch(ctx, msg.Channel, []byte(msg.Payload))
		}
	}
}

// dispatch 解码并把一条 pub/sub 消息分发到 Hub。
func (c *Coordinator) dispatch(ctx context.Context, channel string, payload []byte) {
	switch channel {
	case channelNotifyDirect:
		var env directEnvelope
		if err := json.Unmarshal(payload, &env); err != nil {
			logger.LegacyPrintf("inbox.coordinator", "[Inbox] unmarshal direct notify failed: %v", err)
			return
		}
		c.hub.DeliverDirect(ctx, env.UserID, env.Message)
	case channelNotifyBroadcast:
		var b Broadcast
		if err := json.Unmarshal(payload, &b); err != nil {
			logger.LegacyPrintf("inbox.coordinator", "[Inbox] unmarshal broadcast notify failed: %v", err)
			return
		}
		c.hub.DeliverBroadcast(ctx, b)
	case channelKick:
		var env kickEnvelope
		if err := json.Unmarshal(payload, &env); err != nil {
			logger.LegacyPrintf("inbox.coordinator", "[Inbox] unmarshal kick failed: %v", err)
			return
		}
		c.hub.KickRemote(env.UserID, env.ConnID)
	}
}
