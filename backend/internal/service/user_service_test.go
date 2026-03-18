//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// --- mock: UserRepository ---

type mockUserRepo struct {
	updateBalanceErr error
	updateBalanceFn  func(ctx context.Context, id int64, amount float64) error
}

func (m *mockUserRepo) Create(context.Context, *User) error               { return nil }
func (m *mockUserRepo) GetByID(context.Context, int64) (*User, error)     { return &User{}, nil }
func (m *mockUserRepo) GetByEmail(context.Context, string) (*User, error) { return &User{}, nil }
func (m *mockUserRepo) GetFirstAdmin(context.Context) (*User, error)      { return &User{}, nil }
func (m *mockUserRepo) Update(context.Context, *User) error               { return nil }
func (m *mockUserRepo) Delete(context.Context, int64) error               { return nil }
func (m *mockUserRepo) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockUserRepo) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockUserRepo) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	if m.updateBalanceFn != nil {
		return m.updateBalanceFn(ctx, id, amount)
	}
	return m.updateBalanceErr
}
func (m *mockUserRepo) DeductBalance(context.Context, int64, float64) error { return nil }
func (m *mockUserRepo) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (m *mockUserRepo) ExistsByEmail(context.Context, string) (bool, error) { return false, nil }
func (m *mockUserRepo) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (m *mockUserRepo) AddGroupToAllowedGroups(context.Context, int64, int64) error { return nil }
func (m *mockUserRepo) UpdateTotpSecret(context.Context, int64, *string) error      { return nil }
func (m *mockUserRepo) EnableTotp(context.Context, int64) error                     { return nil }
func (m *mockUserRepo) DisableTotp(context.Context, int64) error                    { return nil }

// --- mock: APIKeyAuthCacheInvalidator ---

type mockAuthCacheInvalidator struct {
	invalidatedUserIDs []int64
	mu                 sync.Mutex
}

func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByKey(context.Context, string)    {}
func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByGroupID(context.Context, int64) {}
func (m *mockAuthCacheInvalidator) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidatedUserIDs = append(m.invalidatedUserIDs, userID)
}

// --- mock: BillingCache ---

type mockBillingCache struct {
	invalidateErr       error
	invalidateCallCount atomic.Int64
	invalidatedUserIDs  []int64
	mu                  sync.Mutex
}

func (m *mockBillingCache) GetUserBalance(context.Context, int64) (float64, error)  { return 0, nil }
func (m *mockBillingCache) SetUserBalance(context.Context, int64, float64) error    { return nil }
func (m *mockBillingCache) DeductUserBalance(context.Context, int64, float64) error { return nil }
func (m *mockBillingCache) InvalidateUserBalance(_ context.Context, userID int64) error {
	m.invalidateCallCount.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidatedUserIDs = append(m.invalidatedUserIDs, userID)
	return m.invalidateErr
}
func (m *mockBillingCache) GetSubscriptionCache(context.Context, int64, int64) (*SubscriptionCacheData, error) {
	return nil, nil
}
func (m *mockBillingCache) SetSubscriptionCache(context.Context, int64, int64, *SubscriptionCacheData) error {
	return nil
}
func (m *mockBillingCache) UpdateSubscriptionUsage(context.Context, int64, int64, float64) error {
	return nil
}
func (m *mockBillingCache) InvalidateSubscriptionCache(context.Context, int64, int64) error {
	return nil
}
func (m *mockBillingCache) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	return nil, nil
}
func (m *mockBillingCache) SetAPIKeyRateLimit(context.Context, int64, *APIKeyRateLimitCacheData) error {
	return nil
}
func (m *mockBillingCache) UpdateAPIKeyRateLimitUsage(context.Context, int64, float64) error {
	return nil
}
func (m *mockBillingCache) InvalidateAPIKeyRateLimit(context.Context, int64) error {
	return nil
}

// --- 测试 ---

func TestUpdateBalance_Success(t *testing.T) {
	repo := &mockUserRepo{}
	cache := &mockBillingCache{}
	svc := NewUserService(repo, nil, cache)

	err := svc.UpdateBalance(context.Background(), 42, 100.0)
	require.NoError(t, err)

	// 等待异步 goroutine 完成
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "应异步调用 InvalidateUserBalance")

	cache.mu.Lock()
	defer cache.mu.Unlock()
	require.Equal(t, []int64{42}, cache.invalidatedUserIDs, "应对 userID=42 失效缓存")
}

func TestUpdateBalance_NilBillingCache_NoPanic(t *testing.T) {
	repo := &mockUserRepo{}
	svc := NewUserService(repo, nil, nil) // billingCache = nil

	err := svc.UpdateBalance(context.Background(), 1, 50.0)
	require.NoError(t, err, "billingCache 为 nil 时不应 panic")
}

func TestUpdateBalance_CacheFailure_DoesNotAffectReturn(t *testing.T) {
	repo := &mockUserRepo{}
	cache := &mockBillingCache{invalidateErr: errors.New("redis connection refused")}
	svc := NewUserService(repo, nil, cache)

	err := svc.UpdateBalance(context.Background(), 99, 200.0)
	require.NoError(t, err, "缓存失效失败不应影响主流程返回值")

	// 等待异步 goroutine 完成（即使失败也应调用）
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "即使失败也应调用 InvalidateUserBalance")
}

func TestUpdateBalance_RepoError_ReturnsError(t *testing.T) {
	repo := &mockUserRepo{updateBalanceErr: errors.New("database error")}
	cache := &mockBillingCache{}
	svc := NewUserService(repo, nil, cache)

	err := svc.UpdateBalance(context.Background(), 1, 100.0)
	require.Error(t, err, "repo 失败时应返回错误")
	require.Contains(t, err.Error(), "update balance")

	// repo 失败时不应触发缓存失效
	time.Sleep(100 * time.Millisecond)
	require.Equal(t, int64(0), cache.invalidateCallCount.Load(),
		"repo 失败时不应调用 InvalidateUserBalance")
}

func TestUpdateBalance_WithAuthCacheInvalidator(t *testing.T) {
	repo := &mockUserRepo{}
	auth := &mockAuthCacheInvalidator{}
	cache := &mockBillingCache{}
	svc := NewUserService(repo, auth, cache)

	err := svc.UpdateBalance(context.Background(), 77, 300.0)
	require.NoError(t, err)

	// 验证 auth cache 同步失效
	auth.mu.Lock()
	require.Equal(t, []int64{77}, auth.invalidatedUserIDs)
	auth.mu.Unlock()

	// 验证 billing cache 异步失效
	require.Eventually(t, func() bool {
		return cache.invalidateCallCount.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestNewUserService_FieldsAssignment(t *testing.T) {
	repo := &mockUserRepo{}
	auth := &mockAuthCacheInvalidator{}
	cache := &mockBillingCache{}

	svc := NewUserService(repo, auth, cache)
	require.NotNil(t, svc)
	require.Equal(t, repo, svc.userRepo)
	require.Equal(t, auth, svc.authCacheInvalidator)
	require.Equal(t, cache, svc.billingCache)
}
