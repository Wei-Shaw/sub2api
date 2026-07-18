//go:build unit

package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// fakeSeq 依次返回预置的 seq 序列。
type fakeSeq struct {
	seqs []int64
	i    int
	err  error
}

func (f *fakeSeq) Next(context.Context) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.i >= len(f.seqs) {
		f.i++
		return int64(1000 + f.i), nil
	}
	s := f.seqs[f.i]
	f.i++
	return s, nil
}

// fakeRepo 只实现发布路径需要的两个 Insert 方法，其余返回未实现。
type fakeRepo struct {
	Repository
	// directResults 按调用次序返回 (created, err)。
	directResults []insertResult
	directCalls   int
	broadcastRes  []insertResult
	broadcastCall int
	lastSeq       []int64
}

type insertResult struct {
	created bool
	err     error
}

func (r *fakeRepo) InsertDirectMessage(_ context.Context, seq int64, _ PublishDirectInput) (bool, error) {
	r.lastSeq = append(r.lastSeq, seq)
	res := r.directResults[r.directCalls]
	r.directCalls++
	return res.created, res.err
}

func (r *fakeRepo) InsertBroadcast(_ context.Context, seq int64, _ PublishBroadcastInput) (bool, error) {
	r.lastSeq = append(r.lastSeq, seq)
	res := r.broadcastRes[r.broadcastCall]
	r.broadcastCall++
	return res.created, res.err
}

// recordingNotifier 记录被通知的 seq。
type recordingNotifier struct {
	direct    []int64
	broadcast []int64
}

func (n *recordingNotifier) NotifyDirect(_ context.Context, _ int64, m Message) {
	n.direct = append(n.direct, m.Seq)
}
func (n *recordingNotifier) NotifyBroadcast(_ context.Context, b Broadcast) {
	n.broadcast = append(n.broadcast, b.Seq)
}

func validDirect() PublishDirectInput {
	return PublishDirectInput{
		RecipientID: 42,
		Namespace:   "support.ticket",
		DedupKey:    "ticket:1:reply:2",
		Payload:     json.RawMessage(`{"ticket_id":1}`),
	}
}

func TestPublishToUser_Success(t *testing.T) {
	repo := &fakeRepo{directResults: []insertResult{{created: true}}}
	notifier := &recordingNotifier{}
	p := NewPublisher(&fakeSeq{seqs: []int64{7}}, repo, notifier)

	created, seq, err := p.PublishToUser(context.Background(), validDirect())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !created || seq != 7 {
		t.Fatalf("got created=%v seq=%d", created, seq)
	}
	if len(notifier.direct) != 1 || notifier.direct[0] != 7 {
		t.Fatalf("notifier 未收到 direct 通知: %v", notifier.direct)
	}
}

func TestPublishToUser_DedupHit_NoNotify(t *testing.T) {
	repo := &fakeRepo{directResults: []insertResult{{created: false}}}
	notifier := &recordingNotifier{}
	p := NewPublisher(&fakeSeq{seqs: []int64{7}}, repo, notifier)

	created, _, err := p.PublishToUser(context.Background(), validDirect())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if created {
		t.Fatal("dedup 命中时 created 应为 false")
	}
	if len(notifier.direct) != 0 {
		t.Fatalf("dedup 命中不应通知: %v", notifier.direct)
	}
}

func TestPublishToUser_SeqConflictRetry(t *testing.T) {
	// 前两次主键冲突，第三次成功。
	repo := &fakeRepo{directResults: []insertResult{
		{err: ErrSeqConflict},
		{err: ErrSeqConflict},
		{created: true},
	}}
	p := NewPublisher(&fakeSeq{seqs: []int64{1, 2, 3}}, repo, nil)

	created, seq, err := p.PublishToUser(context.Background(), validDirect())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !created || seq != 3 {
		t.Fatalf("got created=%v seq=%d", created, seq)
	}
	if repo.directCalls != 3 {
		t.Fatalf("应重试 3 次, got %d", repo.directCalls)
	}
}

func TestPublishToUser_SeqExhausted(t *testing.T) {
	repo := &fakeRepo{directResults: []insertResult{
		{err: ErrSeqConflict}, {err: ErrSeqConflict}, {err: ErrSeqConflict},
	}}
	p := NewPublisher(&fakeSeq{seqs: []int64{1, 2, 3}}, repo, nil)

	_, _, err := p.PublishToUser(context.Background(), validDirect())
	if !errors.Is(err, ErrSeqExhausted) {
		t.Fatalf("应返回 ErrSeqExhausted, got %v", err)
	}
}

func TestPublishToUser_Validation(t *testing.T) {
	p := NewPublisher(&fakeSeq{seqs: []int64{1}}, &fakeRepo{}, nil)
	ctx := context.Background()

	cases := []struct {
		name string
		in   PublishDirectInput
	}{
		{"bad recipient", PublishDirectInput{RecipientID: 0, Namespace: "n", DedupKey: "k", Payload: json.RawMessage(`{}`)}},
		{"empty namespace", PublishDirectInput{RecipientID: 1, Namespace: "", DedupKey: "k", Payload: json.RawMessage(`{}`)}},
		{"bad dedup", PublishDirectInput{RecipientID: 1, Namespace: "n", DedupKey: "bad key!", Payload: json.RawMessage(`{}`)}},
		{"empty payload", PublishDirectInput{RecipientID: 1, Namespace: "n", DedupKey: "k", Payload: nil}},
		{"invalid json", PublishDirectInput{RecipientID: 1, Namespace: "n", DedupKey: "k", Payload: json.RawMessage(`{bad`)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := p.PublishToUser(ctx, c.in); err == nil {
				t.Fatal("期望校验错误")
			}
		})
	}
}

func TestPublishToUser_PayloadTooLarge(t *testing.T) {
	big := make([]byte, MaxPayloadBytes+10)
	for i := range big {
		big[i] = 'a'
	}
	in := validDirect()
	in.Payload = json.RawMessage(`"` + string(big) + `"`)
	p := NewPublisher(&fakeSeq{seqs: []int64{1}}, &fakeRepo{}, nil)
	if _, _, err := p.PublishToUser(context.Background(), in); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("应返回 ErrPayloadTooLarge, got %v", err)
	}
}

func TestPublishBroadcast_Success(t *testing.T) {
	repo := &fakeRepo{broadcastRes: []insertResult{{created: true}}}
	notifier := &recordingNotifier{}
	p := NewPublisher(&fakeSeq{seqs: []int64{9}}, repo, notifier)

	in := PublishBroadcastInput{
		Namespace: "announcement",
		DedupKey:  "ann:5",
		Targeting: json.RawMessage(`{"op":"all_users"}`),
		Payload:   json.RawMessage(`{"title":"hi"}`),
	}
	created, seq, err := p.PublishBroadcast(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !created || seq != 9 {
		t.Fatalf("got created=%v seq=%d", created, seq)
	}
	if len(notifier.broadcast) != 1 || notifier.broadcast[0] != 9 {
		t.Fatalf("notifier 未收到 broadcast 通知: %v", notifier.broadcast)
	}
}

func TestPublishBroadcast_InvalidTargeting(t *testing.T) {
	p := NewPublisher(&fakeSeq{seqs: []int64{1}}, &fakeRepo{}, nil)
	in := PublishBroadcastInput{
		Namespace: "announcement",
		DedupKey:  "ann:5",
		Targeting: json.RawMessage(`{"op":"nope"}`),
		Payload:   json.RawMessage(`{"title":"hi"}`),
	}
	if _, _, err := p.PublishBroadcast(context.Background(), in); !errors.Is(err, ErrInvalidTargeting) {
		t.Fatalf("应返回 ErrInvalidTargeting, got %v", err)
	}
}
