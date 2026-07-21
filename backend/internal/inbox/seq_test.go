//go:build unit

package inbox

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestSeqAllocator(t *testing.T) (*SeqAllocator, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("启动 miniredis 失败: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewSeqAllocator(rdb), mr
}

func TestSeqAllocator_Monotonic(t *testing.T) {
	alloc, _ := newTestSeqAllocator(t)
	ctx := context.Background()

	var prev int64
	for i := 0; i < 1000; i++ {
		seq, err := alloc.Next(ctx)
		if err != nil {
			t.Fatalf("第 %d 次分配失败: %v", i, err)
		}
		if seq <= prev {
			t.Fatalf("seq 非单调递增: prev=%d cur=%d (i=%d)", prev, seq, i)
		}
		prev = seq
	}
}

func TestSeqAllocator_Positive(t *testing.T) {
	alloc, _ := newTestSeqAllocator(t)
	seq, err := alloc.Next(context.Background())
	if err != nil {
		t.Fatalf("分配失败: %v", err)
	}
	if seq <= 0 {
		t.Fatalf("seq 应为正数, got %d", seq)
	}
}

func TestSeqAllocator_EncodingLayout(t *testing.T) {
	alloc, _ := newTestSeqAllocator(t)
	seq, err := alloc.Next(context.Background())
	if err != nil {
		t.Fatalf("分配失败: %v", err)
	}
	// 高位应为毫秒时间戳（> 2020-01-01 的毫秒数），低 SeqTimestampShift 位为计数。
	ms := seq >> SeqTimestampShift
	const msAt2020 = 1577836800000 // 2020-01-01T00:00:00Z
	if ms < msAt2020 {
		t.Fatalf("seq 高位时间戳异常: ms=%d", ms)
	}
}

func TestSeqAllocator_NilClient(t *testing.T) {
	var a *SeqAllocator
	if _, err := a.Next(context.Background()); err == nil {
		t.Fatal("nil allocator 应返回错误")
	}
}
