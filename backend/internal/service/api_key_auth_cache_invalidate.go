package service

import (
	"context"
	"fmt"
)

// ── Strict invalidator interface ─────────────────────────────────────────────
// StrictAPIKeyAuthCacheInvalidator provides strict (error-returning) cache invalidation
// for use by CacheInvalidationOutboxWorker.  Any failure is returned as an error,
// unlike the best-effort methods below that silently absorb errors.
type StrictAPIKeyAuthCacheInvalidator interface {
	// InvalidateAuthCacheByUserIDStrict invalidates the auth cache for all API keys
	// belonging to userID.  Returns an error if the key listing or deletion fails.
	InvalidateAuthCacheByUserIDStrict(ctx context.Context, userID int64) error

	// InvalidateAuthCacheByUserIDsStrict invalidates auth cache for multiple users.
	// All errors are collected; partial failures are still returned.
	InvalidateAuthCacheByUserIDsStrict(ctx context.Context, userIDs []int64) error

	// InvalidateAuthCacheByCacheKeysStrict deletes L1/L2 auth cache entries identified
	// by the already-hashed cache keys (sha256 hex strings from EventPayload.AuthCacheKeys).
	// Does NOT hash the keys again — they must be pre-hashed.
	InvalidateAuthCacheByCacheKeysStrict(ctx context.Context, cacheKeys []string) error
}

// UserGroupRateCacheInvalidator provides error-returning rate cache invalidation
// for (user_id, group_id) pairs stored in the GatewayService in-process gocache.
type UserGroupRateCacheInvalidator interface {
	// InvalidateUserGroupRateCache removes cached rate multiplier entries for the given
	// (user_id, group_id) pairs.  Returns an error if any entry could not be invalidated.
	InvalidateUserGroupRateCache(ctx context.Context, pairs []RatePair) error
}

// InvalidateAuthCacheByKey 清除指定 API Key 的认证缓存
func (s *APIKeyService) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	if key == "" {
		return
	}
	cacheKey := s.authCacheKey(key)
	s.deleteAuthCache(ctx, cacheKey)
}

// InvalidateAuthCacheByUserID 清除用户相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	if userID <= 0 {
		return
	}
	keys, err := s.apiKeyRepo.ListKeysByUserID(ctx, userID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByKeys(ctx, keys)
}

// InvalidateAuthCacheByGroupID 清除分组相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	if groupID <= 0 {
		return
	}
	keys, err := s.apiKeyRepo.ListKeysByGroupID(ctx, groupID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByKeys(ctx, keys)
}

func (s *APIKeyService) deleteAuthCacheByKeys(ctx context.Context, keys []string) {
	if len(keys) == 0 {
		return
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		s.deleteAuthCache(ctx, s.authCacheKey(key))
	}
}

// ── StrictAPIKeyAuthCacheInvalidator implementation ──────────────────────────

// InvalidateAuthCacheByUserIDStrict invalidates auth cache for all API keys belonging
// to userID.  Returns an error if the key listing fails; cache deletions are best-effort.
func (s *APIKeyService) InvalidateAuthCacheByUserIDStrict(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return nil
	}
	keys, err := s.apiKeyRepo.ListKeysByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("list keys for user %d: %w", userID, err)
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		s.deleteAuthCache(ctx, s.authCacheKey(key))
	}
	return nil
}

// InvalidateAuthCacheByUserIDsStrict invalidates auth cache for multiple users.
// Collects all errors; partial failures are still returned.
func (s *APIKeyService) InvalidateAuthCacheByUserIDsStrict(ctx context.Context, userIDs []int64) error {
	var errs []error
	for _, userID := range userIDs {
		if err := s.InvalidateAuthCacheByUserIDStrict(ctx, userID); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("invalidate auth cache for %d user(s) failed: %v", len(errs), errs)
	}
	return nil
}

// InvalidateAuthCacheByCacheKeysStrict deletes L1/L2 auth cache entries for the given
// pre-hashed cache keys.  Does NOT hash the keys again.
func (s *APIKeyService) InvalidateAuthCacheByCacheKeysStrict(ctx context.Context, cacheKeys []string) error {
	for _, ck := range cacheKeys {
		if ck == "" {
			continue
		}
		s.deleteAuthCache(ctx, ck)
	}
	return nil
}
