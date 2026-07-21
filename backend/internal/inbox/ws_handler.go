package inbox

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// newConnID 生成一个全局唯一（进程间碰撞概率可忽略）的正整数连接 id，用于跨实例
// 单连接踢出时区分新旧连接。用 crypto/rand 保证跨副本唯一性（进程本地计数器会碰撞）。
func newConnID() int64 {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		// 极端兜底：退化到纳秒时间戳（仍单调，碰撞概率极低）。
		return time.Now().UnixNano()
	}
	return int64(binary.BigEndian.Uint64(b[:]) >> 1) // 清最高位，保证非负
}

// WebSocket 读写超时与缓冲参数。
const (
	wsWriteWait      = 10 * time.Second
	wsPongWait       = 60 * time.Second
	wsPingPeriod     = (wsPongWait * 9) / 10
	wsMaxMessageSize = 4 << 10 // 上行仅有小型 ack 帧，4KB 足够
	wsSendBuffer     = 64      // 单连接下行缓冲，满则丢弃（客户端靠 catchup 补齐）
)

// WSAuthenticator 从 WS 握手请求中解析并校验用户身份。由应用层注入（浏览器 WS 握手
// 无法携带 Authorization header，需从 subprotocol / query 取 token 自行校验）。
type WSAuthenticator interface {
	Authenticate(r *http.Request) (userID int64, err error)
}

// OriginChecker 校验 WS 握手 Origin。命名类型便于依赖注入装配。nil 时使用 gorilla
// 默认（同源）校验。
type OriginChecker func(*http.Request) bool

// WSHandler 处理信箱 WebSocket 连接的建立与生命周期。
type WSHandler struct {
	hub      *Hub
	svc      *Service
	auth     WSAuthenticator
	metrics  Metrics
	upgrader websocket.Upgrader
}

// NewWSHandler 构造 WS handler。checkOrigin 为 nil 时使用默认（同源）校验。
func NewWSHandler(hub *Hub, svc *Service, auth WSAuthenticator, metrics Metrics, checkOrigin OriginChecker) *WSHandler {
	if metrics == nil {
		metrics = NewNoopMetrics()
	}
	return &WSHandler{
		hub:     hub,
		svc:     svc,
		auth:    auth,
		metrics: metrics,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     checkOrigin,
		},
	}
}

// Handle 处理 GET /inbox/ws：鉴权 → 升级 → 注册到 hub → 启动读写泵。
func (h *WSHandler) Handle(c *gin.Context) {
	userID, err := h.auth.Authenticate(c.Request)
	if err != nil || userID <= 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Upgrade 失败时 gorilla 已写响应。
		return
	}

	wc := &wsConn{
		uid:     userID,
		id:      newConnID(),
		conn:    conn,
		send:    make(chan PushMessage, wsSendBuffer),
		closeCh: make(chan struct{}),
		metrics: h.metrics,
	}

	h.hub.Register(wc)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		wc.writePump()
	}()

	// readPump 在当前 goroutine 阻塞运行，返回即表示连接结束。
	wc.readPump(c.Request.Context(), h.svc)

	h.hub.Unregister(wc)
	wc.close()
	wg.Wait()
}

// wsConn 是一条真实 WebSocket 连接，实现 clientConn。
type wsConn struct {
	uid     int64
	id      int64
	conn    *websocket.Conn
	send    chan PushMessage
	closeCh chan struct{}
	once    sync.Once
	metrics Metrics
}

func (w *wsConn) userID() int64 { return w.uid }
func (w *wsConn) connID() int64 { return w.id }

// deliver 非阻塞投递；缓冲满则丢弃并计数（客户端会在下次 catchup 补齐）。
func (w *wsConn) deliver(msg PushMessage) {
	select {
	case w.send <- msg:
	case <-w.closeCh:
	default:
		w.metrics.IncPushDropped()
	}
}

// kick 发送踢出帧并关闭连接。
func (w *wsConn) kick(reason string) {
	select {
	case w.send <- PushMessage{Type: PushTypeKicked, Reason: reason}:
	default:
	}
	w.close()
}

// close 幂等地触发连接关闭。
func (w *wsConn) close() {
	w.once.Do(func() { close(w.closeCh) })
}

// writePump 负责所有对连接的写操作（gorilla 要求写操作单 goroutine 串行）。
func (w *wsConn) writePump() {
	ticker := time.NewTicker(wsPingPeriod)
	defer func() {
		ticker.Stop()
		_ = w.conn.Close()
	}()

	for {
		select {
		case <-w.closeCh:
			_ = w.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			_ = w.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		case msg := <-w.send:
			_ = w.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := w.conn.WriteJSON(msg); err != nil {
				return
			}
			// kicked 帧发出后关闭连接。
			if msg.Type == PushTypeKicked {
				return
			}
		case <-ticker.C:
			_ = w.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := w.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// inboundFrame 是客户端上行帧。目前仅支持 ack。
type inboundFrame struct {
	Type string `json:"type"`
	Seq  int64  `json:"seq"`
}

// readPump 读取上行帧（ack）并维护 pong 心跳，读错误即结束连接。
func (w *wsConn) readPump(ctx context.Context, svc *Service) {
	w.conn.SetReadLimit(wsMaxMessageSize)
	_ = w.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	w.conn.SetPongHandler(func(string) error {
		return w.conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	for {
		select {
		case <-w.closeCh:
			return
		default:
		}

		_, data, err := w.conn.ReadMessage()
		if err != nil {
			return
		}
		var frame inboundFrame
		if err := json.Unmarshal(data, &frame); err != nil {
			continue // 忽略非法帧
		}
		if frame.Type == "ack" && frame.Seq > 0 {
			if err := svc.Ack(ctx, w.uid, frame.Seq); err != nil {
				logger.LegacyPrintf("inbox.ws", "[Inbox] ws ack user=%d seq=%d failed: %v", w.uid, frame.Seq, err)
			}
		}
	}
}
