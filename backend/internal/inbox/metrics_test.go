//go:build unit

package inbox

import (
	"context"
	"testing"
)

func TestCountingMetrics_Snapshot(t *testing.T) {
	m := NewCountingMetrics()
	m.IncPublished(ScopeDirect)
	m.IncPublished(ScopeDirect)
	m.IncPublished(ScopeBroadcast)
	m.IncSeqAllocRetry()
	m.IncCatchup(3)
	m.IncCatchup(2)
	m.IncAck()
	m.IncWSConnected()
	m.IncWSKicked()
	m.IncPushDropped()

	s := m.Snapshot()
	if s.PublishedDirect != 2 || s.PublishedBroadcast != 1 {
		t.Fatalf("published 计数错误: %+v", s)
	}
	if s.SeqAllocRetries != 1 {
		t.Fatalf("seq 重试计数错误: %+v", s)
	}
	if s.Catchups != 2 || s.CatchupMessages != 5 {
		t.Fatalf("catchup 计数错误: %+v", s)
	}
	if s.Acks != 1 || s.WSConnected != 1 || s.WSKicked != 1 || s.PushDropped != 1 {
		t.Fatalf("其它计数错误: %+v", s)
	}
}

// TestPublisher_CountsSeqRetry 验证 seq 主键冲突重试被 Metrics 记录（tasks 3.6）。
func TestPublisher_CountsSeqRetry(t *testing.T) {
	repo := &fakeRepo{directResults: []insertResult{
		{err: ErrSeqConflict},
		{created: true},
	}}
	m := NewCountingMetrics()
	p := NewPublisherWithMetrics(&fakeSeq{seqs: []int64{1, 2}}, repo, nil, m)

	created, _, err := p.PublishToUser(context.Background(), validDirect())
	if err != nil || !created {
		t.Fatalf("最终应成功: created=%v err=%v", created, err)
	}
	s := m.Snapshot()
	if s.SeqAllocRetries != 1 {
		t.Fatalf("应记录 1 次 seq 重试, got %d", s.SeqAllocRetries)
	}
	if s.PublishedDirect != 1 {
		t.Fatalf("应记录 1 次单播发布, got %d", s.PublishedDirect)
	}
}
