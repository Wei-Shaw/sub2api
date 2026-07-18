package inbox

import (
	"context"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// clientConn 抽象 hub 内的一条用户连接，便于对 fan-out 逻辑做单元测试而不依赖真实
// WebSocket。真实实现见 wsConn（ws_handler.go）。
type clientConn interface {
	// userID 返回连接所属用户。
	userID() int64
	// connID 返回全局唯一的连接 id，用于跨实例踢出时区分"新连接自身"与"待踢旧连接"。
	connID() int64
	// deliver 非阻塞投递一帧下行消息；连接缓冲已满时应丢弃（客户端会靠 catchup 补齐）。
	deliver(msg PushMessage)
	// kick 因单连接策略被新连接替换时调用，通知客户端并关闭。
	kick(reason string)
}

// KickBroadcaster 把"某用户新建了连接 connID"广播到所有实例，用于跨实例单连接踢出。
// 由 Coordinator（Redis pub/sub）实现；nil 时退化为仅本实例踢出。
type KickBroadcaster interface {
	PublishKick(ctx context.Context, userID, connID int64)
}

// Hub 维护本节点的在线连接（v1：每用户至多一条连接，新连接踢掉旧连接），并把
// 投递请求 fan-out 到本地连接。跨节点分发由 Coordinator 通过 Redis pub/sub 完成。
type Hub struct {
	svc     *Service
	attrs   AttributeProvider
	metrics Metrics
	kicker  KickBroadcaster // 可空：跨实例踢出广播器

	mu    sync.RWMutex
	conns map[int64]clientConn
}

// SetKicker 注入跨实例踢出广播器。在 Coordinator 构造后调用（解决 Hub↔Coordinator
// 的构造循环）。传 nil 表示仅本实例踢出。
func (h *Hub) SetKicker(k KickBroadcaster) {
	h.kicker = k
}

// NewHub 构造连接中枢。
func NewHub(svc *Service, attrs AttributeProvider, metrics Metrics) *Hub {
	if attrs == nil {
		attrs = NewNoopAttributeProvider()
	}
	if metrics == nil {
		metrics = NewNoopMetrics()
	}
	return &Hub{
		svc:     svc,
		attrs:   attrs,
		metrics: metrics,
		conns:   make(map[int64]clientConn),
	}
}

// Register 登记一条新连接。若该用户已有连接，旧连接被踢出（单连接策略）。
func (h *Hub) Register(c clientConn) {
	if c == nil {
		return
	}
	uid := c.userID()
	h.mu.Lock()
	old := h.conns[uid]
	h.conns[uid] = c
	h.mu.Unlock()

	if old != nil && old != c {
		old.kick("replaced_by_new_connection")
		h.metrics.IncWSKicked()
	}
	h.metrics.IncWSConnected()

	// 跨实例踢出：广播"用户 uid 新建了连接 c.connID()"，其它实例上该用户的旧连接
	// （connID 不同）会被踢。本实例收到自己的广播时因 connID 相同而 no-op。
	if h.kicker != nil {
		h.kicker.PublishKick(context.Background(), uid, c.connID())
	}
}

// KickRemote 处理来自其它实例的踢出广播：若本实例持有该用户连接且其 connID 与新连接
// 不同，则踢出并移除（该用户的权威连接已转移到别的实例）。
func (h *Hub) KickRemote(userID, exceptConnID int64) {
	h.mu.Lock()
	c, ok := h.conns[userID]
	if !ok || c.connID() == exceptConnID {
		h.mu.Unlock()
		return
	}
	delete(h.conns, userID)
	h.mu.Unlock()

	c.kick("replaced_by_new_connection")
	h.metrics.IncWSKicked()
}

// Unregister 注销连接。仅当当前登记的正是该连接时才删除，避免误删已被替换的新连接。
func (h *Hub) Unregister(c clientConn) {
	if c == nil {
		return
	}
	uid := c.userID()
	h.mu.Lock()
	if cur, ok := h.conns[uid]; ok && cur == c {
		delete(h.conns, uid)
	}
	h.mu.Unlock()
}

// OnlineCount 返回本节点在线连接数（监控用）。
func (h *Hub) OnlineCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

// localConn 取用户在本节点的连接（可能为 nil）。
func (h *Hub) localConn(userID int64) clientConn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.conns[userID]
}

// snapshotConns 复制当前连接列表，避免持锁执行耗时的推送构造。
func (h *Hub) snapshotConns() []clientConn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]clientConn, 0, len(h.conns))
	for _, c := range h.conns {
		out = append(out, c)
	}
	return out
}

// DeliverDirect 把一条单播消息推送给其在本节点的连接（若在线）。
func (h *Hub) DeliverDirect(ctx context.Context, userID int64, m Message) {
	c := h.localConn(userID)
	if c == nil {
		return
	}
	push, err := h.svc.BuildPush(ctx, userID, m)
	if err != nil {
		logger.LegacyPrintf("inbox.hub", "[Inbox] build direct push for user %d failed: %v", userID, err)
		return
	}
	c.deliver(push)
}

// DeliverBroadcast 把一条广播消息推送给本节点上所有命中 targeting 的连接。
func (h *Hub) DeliverBroadcast(ctx context.Context, b Broadcast) {
	conns := h.snapshotConns()
	if len(conns) == 0 {
		return
	}
	m := broadcastToMessage(b)
	for _, c := range conns {
		uid := c.userID()
		attrs, err := h.attrs.Fetch(ctx, uid)
		if err != nil {
			logger.LegacyPrintf("inbox.hub", "[Inbox] fetch attrs for user %d failed: %v", uid, err)
			continue
		}
		if !matchTargeting(b.Targeting, attrs) {
			continue
		}
		push, err := h.svc.BuildPush(ctx, uid, m)
		if err != nil {
			logger.LegacyPrintf("inbox.hub", "[Inbox] build broadcast push for user %d failed: %v", uid, err)
			continue
		}
		c.deliver(push)
	}
}
