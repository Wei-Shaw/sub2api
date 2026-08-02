package service

import "context"

// ProxyGroupCacheVersionStore tracks a cross-instance generation counter per
// proxy group so process-local member caches can detect remote invalidation
// without pub/sub.
type ProxyGroupCacheVersionStore interface {
	// BumpGeneration increments and returns the new generation for groupID.
	BumpGeneration(ctx context.Context, groupID int64) (int64, error)
	// GetGeneration returns the current generation (0 if missing).
	GetGeneration(ctx context.Context, groupID int64) (int64, error)
}
