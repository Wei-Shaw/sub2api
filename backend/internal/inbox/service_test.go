//go:build unit

package inbox

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// stubRepo 是 Service 测试用的内存仓储桩，只实现读取侧需要的方法。
type stubRepo struct {
	Repository
	acked      int64
	found      bool
	initCalled bool
	initSeq    int64
	ackCalls   []int64

	direct     []Message
	broadcasts []Broadcast

	unackedDirect []int64
	unackedBcast  []Broadcast
}

func (r *stubRepo) GetInboxState(context.Context, int64) (int64, bool, error) {
	return r.acked, r.found, nil
}
func (r *stubRepo) InitInboxState(_ context.Context, _ int64, seq int64) error {
	r.initCalled = true
	r.initSeq = seq
	r.acked = seq
	r.found = true
	return nil
}
func (r *stubRepo) Ack(_ context.Context, _ int64, seq int64) error {
	r.ackCalls = append(r.ackCalls, seq)
	return nil
}
func (r *stubRepo) ListDirectSince(_ context.Context, _ int64, sinceSeq int64, limit int) ([]Message, error) {
	var out []Message
	for _, m := range r.direct {
		if m.Seq > sinceSeq {
			out = append(out, m)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
func (r *stubRepo) ListBroadcastsSince(_ context.Context, sinceSeq int64, _ time.Time, limit int) ([]Broadcast, error) {
	var out []Broadcast
	for _, b := range r.broadcasts {
		if b.Seq > sinceSeq {
			out = append(out, b)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
func (r *stubRepo) UnackedDirectSeqs(_ context.Context, _ int64, ackedSeq, _ int64, limit int) ([]int64, error) {
	var out []int64
	for _, s := range r.unackedDirect {
		if s > ackedSeq {
			out = append(out, s)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
func (r *stubRepo) UnackedBroadcasts(_ context.Context, ackedSeq, _ int64, _ time.Time, limit int) ([]Broadcast, error) {
	var out []Broadcast
	for _, b := range r.unackedBcast {
		if b.Seq > ackedSeq {
			out = append(out, b)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func allUsers() json.RawMessage { return json.RawMessage(`{"op":"all_users"}`) }
func adminOnly() json.RawMessage {
	return json.RawMessage(`{"op":"equals","attr":"role","value":"admin"}`)
}

func newSvc(repo Repository, seq SeqSource, attrs AttributeProvider) *Service {
	return NewService(repo, seq, attrs, nil, Config{})
}

func TestCatchup_LazyInit_NewUser(t *testing.T) {
	repo := &stubRepo{found: false}
	svc := newSvc(repo, &fakeSeq{seqs: []int64{500}}, NewNoopAttributeProvider())

	res, err := svc.Catchup(context.Background(), 1, 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !repo.initCalled || repo.initSeq != 500 {
		t.Fatalf("应懒初始化水位=500, got initCalled=%v seq=%d", repo.initCalled, repo.initSeq)
	}
	if res.AckedSeq != 500 {
		t.Fatalf("AckedSeq 应为 500, got %d", res.AckedSeq)
	}
	if len(res.Messages) != 0 {
		t.Fatalf("新用户不应有历史消息, got %d", len(res.Messages))
	}
}

func TestCatchup_MergesAndFiltersBroadcast(t *testing.T) {
	repo := &stubRepo{
		found: true,
		acked: 10,
		direct: []Message{
			{Seq: 11, Scope: ScopeDirect, Namespace: "support.ticket"},
			{Seq: 15, Scope: ScopeDirect, Namespace: "support.ticket"},
		},
		broadcasts: []Broadcast{
			{Seq: 12, Namespace: "announcement", Targeting: allUsers()},
			{Seq: 13, Namespace: "announcement", Targeting: adminOnly()}, // 非 admin 不命中
		},
	}
	// 普通用户属性 role=user。
	attrs := AttributeProviderFunc(func(context.Context, int64) (map[string]any, error) {
		return map[string]any{"role": "user"}, nil
	})
	svc := newSvc(repo, nil, attrs)

	res, err := svc.Catchup(context.Background(), 7, 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// since = max(7, acked=10) = 10 → seq 11,12,15 命中；13 因 targeting 被过滤。
	wantSeqs := []int64{11, 12, 15}
	if len(res.Messages) != len(wantSeqs) {
		t.Fatalf("期望 %d 条, got %d: %+v", len(wantSeqs), len(res.Messages), res.Messages)
	}
	for i, m := range res.Messages {
		if m.Seq != wantSeqs[i] {
			t.Fatalf("消息顺序错误: got %+v", res.Messages)
		}
	}
}

func TestCatchup_AdminMatchesBroadcast(t *testing.T) {
	repo := &stubRepo{
		found:      true,
		acked:      10,
		broadcasts: []Broadcast{{Seq: 13, Namespace: "announcement", Targeting: adminOnly()}},
	}
	attrs := AttributeProviderFunc(func(context.Context, int64) (map[string]any, error) {
		return map[string]any{"role": "admin"}, nil
	})
	svc := newSvc(repo, nil, attrs)

	res, err := svc.Catchup(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Messages) != 1 || res.Messages[0].Seq != 13 {
		t.Fatalf("admin 应命中广播 13, got %+v", res.Messages)
	}
}

func TestCatchup_HasMore(t *testing.T) {
	repo := &stubRepo{found: true, acked: 0}
	// 制造超过 CatchupLimit 的消息。
	for i := int64(1); i <= defaultCatchupLimit+5; i++ {
		repo.direct = append(repo.direct, Message{Seq: i, Scope: ScopeDirect})
	}
	svc := newSvc(repo, nil, NewNoopAttributeProvider())

	res, err := svc.Catchup(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.HasMore {
		t.Fatal("应标记 HasMore")
	}
	if len(res.Messages) != defaultCatchupLimit {
		t.Fatalf("应截断到 %d, got %d", defaultCatchupLimit, len(res.Messages))
	}
}

func TestAck(t *testing.T) {
	repo := &stubRepo{found: true}
	svc := newSvc(repo, nil, nil)

	if err := svc.Ack(context.Background(), 1, 42); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(repo.ackCalls) != 1 || repo.ackCalls[0] != 42 {
		t.Fatalf("ack 未透传 seq, got %v", repo.ackCalls)
	}
	if err := svc.Ack(context.Background(), 1, 0); err == nil {
		t.Fatal("seq<=0 应返回错误")
	}
}

func TestUnreadCount(t *testing.T) {
	repo := &stubRepo{
		found:         true,
		acked:         5,
		unackedDirect: []int64{6, 7, 8},
		unackedBcast: []Broadcast{
			{Seq: 9, Targeting: allUsers()},
			{Seq: 10, Targeting: adminOnly()},
		},
	}
	attrs := AttributeProviderFunc(func(context.Context, int64) (map[string]any, error) {
		return map[string]any{"role": "user"}, nil
	})
	svc := newSvc(repo, nil, attrs)

	count, truncated, err := svc.UnreadCount(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 6,7,8 (direct) + 9 (all_users)；10 被过滤 → 4。
	if count != 4 || truncated {
		t.Fatalf("期望 count=4 truncated=false, got count=%d truncated=%v", count, truncated)
	}
}

func TestUnreadCount_NewUserZero(t *testing.T) {
	repo := &stubRepo{found: false}
	svc := newSvc(repo, nil, nil)
	count, _, err := svc.UnreadCount(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if count != 0 {
		t.Fatalf("新用户未读应为 0, got %d", count)
	}
}

func TestBuildPush(t *testing.T) {
	repo := &stubRepo{found: true, acked: 5, unackedDirect: []int64{6, 7}}
	svc := newSvc(repo, nil, NewNoopAttributeProvider())

	push, err := svc.BuildPush(context.Background(), 1, Message{Seq: 7, Scope: ScopeDirect, Namespace: "support.ticket"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if push.Type != PushTypeNotification || push.Seq != 7 {
		t.Fatalf("push 帧异常: %+v", push)
	}
	if len(push.Unacked) != 2 {
		t.Fatalf("unacked 应为 [6,7], got %v", push.Unacked)
	}
}
