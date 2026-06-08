//go:build unit

// API Key 服务删除方法的单元测试
// 测试 APIKeyService.Delete 方法在各种场景下的行为，
// 包括权限验证、缓存清理和错误处理

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// apiKeyRepoStub 是 APIKeyRepository 接口的测试桩实现。
// 用于隔离测试 APIKeyService.Delete 方法，避免依赖真实数据库。
//
// 设计说明：
//   - apiKey/getByIDErr: 模拟 GetKeyAndOwnerID 返回的记录与错误
//   - deleteErr: 模拟 Delete 返回的错误
//   - deletedIDs: 记录被调用删除的 API Key ID，用于断言验证
type apiKeyRepoStub struct {
	apiKey         *APIKey // GetKeyAndOwnerID 的返回值
	getByIDErr     error   // GetKeyAndOwnerID 的错误返回值
	deleteErr      error   // Delete 的错误返回值
	deletedIDs     []int64 // 记录已删除的 API Key ID 列表
	updateLastUsed func(ctx context.Context, id int64, usedAt time.Time) error
	touchedIDs     []int64
	touchedUsedAts []time.Time
}

// 以下方法在本测试中不应被调用，使用 panic 确保测试失败时能快速定位问题

func (s *apiKeyRepoStub) Create(ctx context.Context, key *APIKey) error {
	panic("unexpected Create call")
}

func (s *apiKeyRepoStub) GetByID(ctx context.Context, id int64) (*APIKey, error) {
	if s.getByIDErr != nil {
		return nil, s.getByIDErr
	}
	if s.apiKey != nil {
		clone := *s.apiKey
		return &clone, nil
	}
	panic("unexpected GetByID call")
}

func (s *apiKeyRepoStub) GetKeyAndOwnerID(ctx context.Context, id int64) (string, int64, error) {
	if s.getByIDErr != nil {
		return "", 0, s.getByIDErr
	}
	if s.apiKey != nil {
		return s.apiKey.Key, s.apiKey.UserID, nil
	}
	return "", 0, ErrAPIKeyNotFound
}

func (s *apiKeyRepoStub) GetByKey(ctx context.Context, key string) (*APIKey, error) {
	panic("unexpected GetByKey call")
}

func (s *apiKeyRepoStub) GetByKeyForAuth(ctx context.Context, key string) (*APIKey, error) {
	panic("unexpected GetByKeyForAuth call")
}

func (s *apiKeyRepoStub) Update(ctx context.Context, key *APIKey) error {
	panic("unexpected Update call")
}

// Delete 记录被删除的 API Key ID 并返回预设的错误。
// 通过 deletedIDs 可以验证删除操作是否被正确调用。
func (s *apiKeyRepoStub) Delete(ctx context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	return s.deleteErr
}

// 以下是接口要求实现但本测试不关心的方法

func (s *apiKeyRepoStub) ListByUserID(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserID call")
}

func (s *apiKeyRepoStub) VerifyOwnership(ctx context.Context, userID int64, apiKeyIDs []int64) ([]int64, error) {
	panic("unexpected VerifyOwnership call")
}

func (s *apiKeyRepoStub) CountByUserID(ctx context.Context, userID int64) (int64, error) {
	panic("unexpected CountByUserID call")
}

func (s *apiKeyRepoStub) ExistsByKey(ctx context.Context, key string) (bool, error) {
	panic("unexpected ExistsByKey call")
}

func (s *apiKeyRepoStub) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}

func (s *apiKeyRepoStub) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]APIKey, error) {
	panic("unexpected SearchAPIKeys call")
}

func (s *apiKeyRepoStub) ClearGroupIDByGroupID(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected ClearGroupIDByGroupID call")
}
func (s *apiKeyRepoStub) UpdateGroupIDByUserAndGroup(ctx context.Context, userID, oldGroupID, newGroupID int64) (int64, error) {
	panic("unexpected UpdateGroupIDByUserAndGroup call")
}

func (s *apiKeyRepoStub) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected CountByGroupID call")
}

func (s *apiKeyRepoStub) ListKeysByUserID(ctx context.Context, userID int64) ([]string, error) {
	panic("unexpected ListKeysByUserID call")
}

func (s *apiKeyRepoStub) ListKeysByGroupID(ctx context.Context, groupID int64) ([]string, error) {
	panic("unexpected ListKeysByGroupID call")
}

func (s *apiKeyRepoStub) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) (float64, error) {
	panic("unexpected IncrementQuotaUsed call")
}

func (s *apiKeyRepoStub) UpdateLastUsed(ctx context.Context, id int64, usedAt time.Time) error {
	s.touchedIDs = append(s.touchedIDs, id)
	s.touchedUsedAts = append(s.touchedUsedAts, usedAt)
	if s.updateLastUsed != nil {
		return s.updateLastUsed(ctx, id, usedAt)
	}
	return nil
}

func (s *apiKeyRepoStub) IncrementRateLimitUsage(ctx context.Context, id int64, cost float64) error {
	panic("unexpected IncrementRateLimitUsage call")
}

func (s *apiKeyRepoStub) ResetRateLimitWindows(ctx context.Context, id int64) error {
	panic("unexpected ResetRateLimitWindows call")
}

func (s *apiKeyRepoStub) GetRateLimitData(ctx context.Context, id int64) (*APIKeyRateLimitData, error) {
	panic("unexpected GetRateLimitData call")
}

// apiKeyCacheStub 是 APIKeyCache 接口的测试桩实现。
// 用于验证删除操作时缓存清理逻辑是否被正确调用。
//
// 设计说明：
//   - invalidated: 记录被清除缓存的用户 ID 列表
type apiKeyCacheStub struct {
	invalidated    []int64  // 记录调用 DeleteCreateAttemptCount 时传入的用户 ID
	deleteAuthKeys []string // 记录调用 DeleteAuthCache 时传入的缓存 key
}

// GetCreateAttemptCount 返回 0，表示用户未超过创建次数限制
func (s *apiKeyCacheStub) GetCreateAttemptCount(ctx context.Context, userID int64) (int, error) {
	return 0, nil
}

// IncrementCreateAttemptCount 空实现，本测试不验证此行为
func (s *apiKeyCacheStub) IncrementCreateAttemptCount(ctx context.Context, userID int64) error {
	return nil
}

// DeleteCreateAttemptCount 记录被清除缓存的用户 ID。
// 删除 API Key 时会调用此方法清除用户的创建尝试计数缓存。
func (s *apiKeyCacheStub) DeleteCreateAttemptCount(ctx context.Context, userID int64) error {
	s.invalidated = append(s.invalidated, userID)
	return nil
}

// IncrementDailyUsage 空实现，本测试不验证此行为
func (s *apiKeyCacheStub) IncrementDailyUsage(ctx context.Context, apiKey string) error {
	return nil
}

// SetDailyUsageExpiry 空实现，本测试不验证此行为
func (s *apiKeyCacheStub) SetDailyUsageExpiry(ctx context.Context, apiKey string, ttl time.Duration) error {
	return nil
}

func (s *apiKeyCacheStub) GetAuthCache(ctx context.Context, key string) (*APIKeyAuthCacheEntry, error) {
	return nil, nil
}

func (s *apiKeyCacheStub) SetAuthCache(ctx context.Context, key string, entry *APIKeyAuthCacheEntry, ttl time.Duration) error {
	return nil
}

func (s *apiKeyCacheStub) DeleteAuthCache(ctx context.Context, key string) error {
	s.deleteAuthKeys = append(s.deleteAuthKeys, key)
	return nil
}

func (s *apiKeyCacheStub) PublishAuthCacheInvalidation(ctx context.Context, cacheKey string) error {
	return nil
}

func (s *apiKeyCacheStub) SubscribeAuthCacheInvalidation(ctx context.Context, handler func(cacheKey string)) error {
	return nil
}

// TestApiKeyService_Delete_OwnerMismatch 测试非所有者尝试删除时返回权限错误。
// 预期行为：
//   - GetKeyAndOwnerID 返回所有者 ID 为 1
//   - 调用者 userID 为 2（不匹配）
//   - 返回 ErrInsufficientPerms 错误
//   - Delete 方法不被调用
//   - 缓存不被清除
func TestApiKeyService_Delete_OwnerMismatch(t *testing.T) {
	repo := &apiKeyRepoStub{
		apiKey: &APIKey{ID: 10, UserID: 1, Key: "k"},
	}
	cache := &apiKeyCacheStub{}
	svc := &APIKeyService{apiKeyRepo: repo, cache: cache}

	err := svc.Delete(context.Background(), 10, 2) // API Key ID=10, 调用者 userID=2
	require.ErrorIs(t, err, ErrInsufficientPerms)
	require.Empty(t, repo.deletedIDs)   // 验证删除操作未被调用
	require.Empty(t, cache.invalidated) // 验证缓存未被清除
	require.Empty(t, cache.deleteAuthKeys)
}

// TestApiKeyService_Delete_Success 测试所有者成功删除 API Key 的场景。
// 预期行为：
//   - GetKeyAndOwnerID 返回所有者 ID 为 7
//   - 调用者 userID 为 7（匹配）
//   - Delete 成功执行
//   - 缓存被正确清除（使用 ownerID）
//   - 返回 nil 错误
func TestApiKeyService_Delete_Success(t *testing.T) {
	repo := &apiKeyRepoStub{
		apiKey: &APIKey{ID: 42, UserID: 7, Key: "k"},
	}
	cache := &apiKeyCacheStub{}
	svc := &APIKeyService{apiKeyRepo: repo, cache: cache}
	svc.lastUsedTouchL1.Store(int64(42), time.Now())

	err := svc.Delete(context.Background(), 42, 7) // API Key ID=42, 调用者 userID=7
	require.NoError(t, err)
	require.Equal(t, []int64{42}, repo.deletedIDs)  // 验证正确的 API Key 被删除
	require.Equal(t, []int64{7}, cache.invalidated) // 验证所有者的缓存被清除
	require.Equal(t, []string{svc.authCacheKey("k")}, cache.deleteAuthKeys)
	_, exists := svc.lastUsedTouchL1.Load(int64(42))
	require.False(t, exists, "delete should clear touch debounce cache")
}

// TestApiKeyService_Delete_NotFound 测试删除不存在的 API Key 时返回正确的错误。
// 预期行为：
//   - GetKeyAndOwnerID 返回 ErrAPIKeyNotFound 错误
//   - 返回 ErrAPIKeyNotFound 错误（被 fmt.Errorf 包装）
//   - Delete 方法不被调用
//   - 缓存不被清除
func TestApiKeyService_Delete_NotFound(t *testing.T) {
	repo := &apiKeyRepoStub{getByIDErr: ErrAPIKeyNotFound}
	cache := &apiKeyCacheStub{}
	svc := &APIKeyService{apiKeyRepo: repo, cache: cache}

	err := svc.Delete(context.Background(), 99, 1)
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	require.Empty(t, repo.deletedIDs)
	require.Empty(t, cache.invalidated)
	require.Empty(t, cache.deleteAuthKeys)
}

// TestApiKeyService_Delete_DeleteFails 测试删除操作失败时的错误处理。
// 预期行为：
//   - GetKeyAndOwnerID 返回正确的所有者 ID
//   - 所有权验证通过
//   - 缓存被清除（在删除之前）
//   - Delete 被调用但返回错误
//   - 返回包含 "delete api key" 的错误信息
func TestApiKeyService_Delete_DeleteFails(t *testing.T) {
	repo := &apiKeyRepoStub{
		apiKey:    &APIKey{ID: 42, UserID: 3, Key: "k"},
		deleteErr: errors.New("delete failed"),
	}
	cache := &apiKeyCacheStub{}
	svc := &APIKeyService{apiKeyRepo: repo, cache: cache}

	err := svc.Delete(context.Background(), 3, 3) // API Key ID=3, 调用者 userID=3
	require.Error(t, err)
	require.ErrorContains(t, err, "delete api key")
	require.Equal(t, []int64{3}, repo.deletedIDs)   // 验证删除操作被调用
	require.Equal(t, []int64{3}, cache.invalidated) // 验证缓存已被清除（即使删除失败）
	require.Equal(t, []string{svc.authCacheKey("k")}, cache.deleteAuthKeys)
}
