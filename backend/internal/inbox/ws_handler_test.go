//go:build unit

package inbox

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// stubWSAuth 从 ?uid= 解析用户；?bad=1 模拟鉴权失败。
type stubWSAuth struct{}

func (stubWSAuth) Authenticate(r *http.Request) (int64, error) {
	if r.URL.Query().Get("bad") == "1" {
		return 0, errors.New("bad token")
	}
	uid, _ := strconv.ParseInt(r.URL.Query().Get("uid"), 10, 64)
	if uid <= 0 {
		return 0, errors.New("no uid")
	}
	return uid, nil
}

// wsAckRepo 在 stubRepo 基础上用 channel 线程安全地记录 ack（避免与测试读发生竞态）。
type wsAckRepo struct {
	stubRepo
	ackCh chan int64
}

func (r *wsAckRepo) Ack(_ context.Context, _ int64, seq int64) error {
	select {
	case r.ackCh <- seq:
	default:
	}
	return nil
}

func newWSTestServer(t *testing.T, svc *Service) (*httptest.Server, *Hub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	hub := NewHub(svc, NewNoopAttributeProvider(), nil)
	ws := NewWSHandler(hub, svc, stubWSAuth{}, nil, func(*http.Request) bool { return true })
	r := gin.New()
	r.GET("/ws", ws.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, hub
}

func wsDial(t *testing.T, srv *httptest.Server, query string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?" + query
	return websocket.DefaultDialer.Dial(url, nil)
}

func waitFor(t *testing.T, msg string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待超时: %s", msg)
}

func TestWS_RejectsInvalidToken(t *testing.T) {
	svc := NewService(&stubRepo{found: true}, &fakeSeq{seqs: []int64{1}}, NewNoopAttributeProvider(), nil, Config{})
	srv, _ := newWSTestServer(t, svc)

	conn, resp, err := wsDial(t, srv, "bad=1")
	if err == nil {
		_ = conn.Close()
		t.Fatal("非法 token 应拒绝升级")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("应返回 401, got resp=%v", resp)
	}
}

func TestWS_PushAndAck(t *testing.T) {
	repo := &wsAckRepo{
		stubRepo: stubRepo{found: true, acked: 5, unackedDirect: []int64{6}},
		ackCh:    make(chan int64, 4),
	}
	svc := NewService(repo, &fakeSeq{seqs: []int64{1}}, NewNoopAttributeProvider(), nil, Config{})
	srv, hub := newWSTestServer(t, svc)

	conn, _, err := wsDial(t, srv, "uid=1")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// 等 hub 注册完成。
	waitFor(t, "连接注册到 hub", func() bool { return hub.OnlineCount() == 1 })

	// 服务端投递一条单播 → 客户端应收到 notification 帧。
	hub.DeliverDirect(context.Background(), 1, Message{Seq: 6, Scope: ScopeDirect, Namespace: "support_ticket"})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var frame PushMessage
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("read push: %v", err)
	}
	if frame.Type != PushTypeNotification || frame.Seq != 6 {
		t.Fatalf("推送帧异常: %+v", frame)
	}

	// 客户端上行 ack → 服务端应调用 svc.Ack。
	if err := conn.WriteJSON(inboundFrame{Type: "ack", Seq: 6}); err != nil {
		t.Fatalf("write ack: %v", err)
	}
	select {
	case seq := <-repo.ackCh:
		if seq != 6 {
			t.Fatalf("ack seq 应为 6, got %d", seq)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("等待服务端 ack 超时")
	}
}

func TestWS_KicksOldConnection(t *testing.T) {
	svc := NewService(&stubRepo{found: true}, &fakeSeq{seqs: []int64{1}}, NewNoopAttributeProvider(), nil, Config{})
	srv, hub := newWSTestServer(t, svc)

	conn1, _, err := wsDial(t, srv, "uid=7")
	if err != nil {
		t.Fatalf("dial1: %v", err)
	}
	defer func() { _ = conn1.Close() }()
	waitFor(t, "conn1 注册", func() bool { return hub.OnlineCount() == 1 })

	// 第二条连接（同一用户）应踢掉第一条。
	conn2, _, err := wsDial(t, srv, "uid=7")
	if err != nil {
		t.Fatalf("dial2: %v", err)
	}
	defer func() { _ = conn2.Close() }()

	// conn1 应收到 kicked 帧。
	_ = conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	var frame PushMessage
	if err := conn1.ReadJSON(&frame); err != nil {
		t.Fatalf("conn1 应收到 kicked 帧, read err: %v", err)
	}
	if frame.Type != PushTypeKicked {
		t.Fatalf("应为 kicked 帧, got %+v", frame)
	}
}
