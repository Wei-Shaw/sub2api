//go:build unit

package inbox

import (
	"context"
	"testing"
	"time"
)

// cleanupRepo 模拟分批删除：每批最多删 batch 行，直到清空。
type cleanupRepo struct {
	Repository
	directRemaining    int64
	broadcastRemaining int64
}

func (r *cleanupRepo) DeleteExpiredDirect(_ context.Context, _ time.Time, limit int) (int64, error) {
	return takeBatch(&r.directRemaining, limit), nil
}
func (r *cleanupRepo) DeleteExpiredBroadcasts(_ context.Context, _ time.Time, limit int) (int64, error) {
	return takeBatch(&r.broadcastRemaining, limit), nil
}

func takeBatch(remaining *int64, limit int) int64 {
	if *remaining <= 0 {
		return 0
	}
	n := int64(limit)
	if *remaining < n {
		n = *remaining
	}
	*remaining -= n
	return n
}

func TestCleaner_RunOnce_BatchesUntilEmpty(t *testing.T) {
	repo := &cleanupRepo{directRemaining: 25, broadcastRemaining: 7}
	c := NewCleaner(repo, defaultRetention, 10)

	d, b, err := c.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if d != 25 || b != 7 {
		t.Fatalf("应删除 direct=25 broadcast=7, got %d/%d", d, b)
	}
	if repo.directRemaining != 0 || repo.broadcastRemaining != 0 {
		t.Fatalf("应清空, 剩余 %d/%d", repo.directRemaining, repo.broadcastRemaining)
	}
}

func TestCleaner_RunOnce_Empty(t *testing.T) {
	repo := &cleanupRepo{}
	c := NewCleaner(repo, 0, 0) // 用默认值
	d, b, err := c.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if d != 0 || b != 0 {
		t.Fatalf("空表应删除 0, got %d/%d", d, b)
	}
}
