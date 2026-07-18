//go:build unit

package inbox

import (
	"context"
	"sync"
	"testing"
)

// fakeConn 是 clientConn 的测试实现，记录投递与踢出。
type fakeConn struct {
	uid      int64
	id       int64
	mu       sync.Mutex
	received []PushMessage
	kicked   bool
	kickMsg  string
}

func (c *fakeConn) userID() int64 { return c.uid }
func (c *fakeConn) connID() int64 { return c.id }
func (c *fakeConn) deliver(msg PushMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.received = append(c.received, msg)
}
func (c *fakeConn) kick(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kicked = true
	c.kickMsg = reason
}
func (c *fakeConn) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.received)
}

func newHubWithSvc(repo Repository, attrs AttributeProvider) *Hub {
	svc := NewService(repo, nil, attrs, nil, Config{})
	return NewHub(svc, attrs, nil)
}

func TestHub_RegisterKicksOld(t *testing.T) {
	h := newHubWithSvc(&stubRepo{found: true}, NewNoopAttributeProvider())
	old := &fakeConn{uid: 1}
	fresh := &fakeConn{uid: 1}

	h.Register(old)
	h.Register(fresh)

	if !old.kicked {
		t.Fatal("旧连接应被踢出")
	}
	if h.OnlineCount() != 1 {
		t.Fatalf("在线连接应为 1, got %d", h.OnlineCount())
	}
}

func TestHub_UnregisterOnlyIfCurrent(t *testing.T) {
	h := newHubWithSvc(&stubRepo{found: true}, NewNoopAttributeProvider())
	old := &fakeConn{uid: 1}
	fresh := &fakeConn{uid: 1}
	h.Register(old)
	h.Register(fresh)

	// 旧连接注销不应删掉已替换的新连接。
	h.Unregister(old)
	if h.OnlineCount() != 1 {
		t.Fatalf("新连接应仍在线, got %d", h.OnlineCount())
	}
	h.Unregister(fresh)
	if h.OnlineCount() != 0 {
		t.Fatalf("注销后应为 0, got %d", h.OnlineCount())
	}
}

// recordingKicker 记录 PublishKick 调用，验证 Register 触发跨实例踢出广播。
type recordingKicker struct {
	calls []kickEnvelope
}

func (k *recordingKicker) PublishKick(_ context.Context, userID, connID int64) {
	k.calls = append(k.calls, kickEnvelope{UserID: userID, ConnID: connID})
}

func TestHub_RegisterBroadcastsKick(t *testing.T) {
	h := newHubWithSvc(&stubRepo{found: true}, NewNoopAttributeProvider())
	k := &recordingKicker{}
	h.SetKicker(k)

	c := &fakeConn{uid: 1, id: 100}
	h.Register(c)

	if len(k.calls) != 1 || k.calls[0].UserID != 1 || k.calls[0].ConnID != 100 {
		t.Fatalf("Register 应广播 kick{uid:1,conn:100}, got %+v", k.calls)
	}
}

func TestHub_KickRemote(t *testing.T) {
	h := newHubWithSvc(&stubRepo{found: true}, NewNoopAttributeProvider())
	local := &fakeConn{uid: 1, id: 100}
	h.Register(local)

	// 收到"同一 connID"的踢出广播（即自己），不应踢自己。
	h.KickRemote(1, 100)
	if local.kicked {
		t.Fatal("同 connID 不应踢自己")
	}
	if h.OnlineCount() != 1 {
		t.Fatalf("在线数应仍为 1, got %d", h.OnlineCount())
	}

	// 收到"不同 connID"（其它实例新建连接）的踢出广播，应踢本地旧连接并移除。
	h.KickRemote(1, 200)
	if !local.kicked {
		t.Fatal("不同 connID 应踢本地旧连接")
	}
	if h.OnlineCount() != 0 {
		t.Fatalf("踢出后本地在线数应为 0, got %d", h.OnlineCount())
	}

	// 用户不在本实例时，no-op 不 panic。
	h.KickRemote(999, 1)
}

func TestHub_DeliverDirect(t *testing.T) {
	repo := &stubRepo{found: true, acked: 5, unackedDirect: []int64{6}}
	h := newHubWithSvc(repo, NewNoopAttributeProvider())
	c := &fakeConn{uid: 1}
	h.Register(c)

	h.DeliverDirect(context.Background(), 1, Message{Seq: 6, Scope: ScopeDirect, Namespace: "support.ticket"})
	if c.count() != 1 {
		t.Fatalf("应投递 1 帧, got %d", c.count())
	}

	// 离线用户不投递（无 panic）。
	h.DeliverDirect(context.Background(), 999, Message{Seq: 7})
}

func TestHub_DeliverBroadcast_Filtered(t *testing.T) {
	repo := &stubRepo{found: true, acked: 0}
	attrsByUser := map[int64]map[string]any{
		1: {"role": "admin"},
		2: {"role": "user"},
	}
	attrs := AttributeProviderFunc(func(_ context.Context, uid int64) (map[string]any, error) {
		return attrsByUser[uid], nil
	})
	h := newHubWithSvc(repo, attrs)
	admin := &fakeConn{uid: 1}
	user := &fakeConn{uid: 2}
	h.Register(admin)
	h.Register(user)

	h.DeliverBroadcast(context.Background(), Broadcast{Seq: 10, Namespace: "announcement", Targeting: adminOnly()})

	if admin.count() != 1 {
		t.Fatalf("admin 应收到广播, got %d", admin.count())
	}
	if user.count() != 0 {
		t.Fatalf("普通用户不应收到 admin 广播, got %d", user.count())
	}
}
