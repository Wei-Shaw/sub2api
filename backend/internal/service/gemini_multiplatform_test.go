//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// mockAccountRepoForGemini Gemini 测试用的 mock
type mockAccountRepoForGemini struct {
	accounts           []Account
	accountsByID       map[int64]*Account
	listByGroupFunc    func(ctx context.Context, groupID int64, platforms []string) ([]Account, error)
	listByPlatformFunc func(ctx context.Context, platforms []string) ([]Account, error)
}

func (m *mockAccountRepoForGemini) GetByID(ctx context.Context, id int64) (*Account, error) {
	if acc, ok := m.accountsByID[id]; ok {
		return acc, nil
	}
	return nil, errors.New("account not found")
}

func (m *mockAccountRepoForGemini) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	var result []*Account
	for _, id := range ids {
		if acc, ok := m.accountsByID[id]; ok {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (m *mockAccountRepoForGemini) ExistsByID(ctx context.Context, id int64) (bool, error) {
	if m.accountsByID == nil {
		return false, nil
	}
	_, ok := m.accountsByID[id]
	return ok, nil
}

func (m *mockAccountRepoForGemini) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	var result []Account
	for _, acc := range m.accounts {
		if acc.Platform == platform && acc.IsSchedulable() {
			result = append(result, acc)
		}
	}
	return result, nil
}

func (m *mockAccountRepoForGemini) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	// 测试时不区分 groupID，直接按 platform 过滤
	return m.ListSchedulableByPlatform(ctx, platform)
}

// Stub methods to implement AccountRepository interface
func (m *mockAccountRepoForGemini) Create(ctx context.Context, account *Account) error { return nil }
func (m *mockAccountRepoForGemini) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*Account, error) {
	return nil, nil
}

func (m *mockAccountRepoForGemini) FindByExtraField(ctx context.Context, key string, value any) ([]Account, error) {
	return nil, nil
}

func (m *mockAccountRepoForGemini) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) Update(ctx context.Context, account *Account) error { return nil }
func (m *mockAccountRepoForGemini) Delete(ctx context.Context, id int64) error         { return nil }
func (m *mockAccountRepoForGemini) List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockAccountRepoForGemini) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockAccountRepoForGemini) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListActive(ctx context.Context) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) UpdateLastUsed(ctx context.Context, id int64) error { return nil }
func (m *mockAccountRepoForGemini) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetError(ctx context.Context, id int64, errorMsg string) error {
	return nil
}
func (m *mockAccountRepoForGemini) ClearError(ctx context.Context, id int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return nil
}
func (m *mockAccountRepoForGemini) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
}
func (m *mockAccountRepoForGemini) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) ListSchedulable(ctx context.Context) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error) {
	return nil, nil
}
func (m *mockAccountRepoForGemini) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	if m.listByPlatformFunc != nil {
		return m.listByPlatformFunc(ctx, platforms)
	}
	var result []Account
	platformSet := make(map[string]bool)
	for _, p := range platforms {
		platformSet[p] = true
	}
	for _, acc := range m.accounts {
		if platformSet[acc.Platform] && acc.IsSchedulable() {
			result = append(result, acc)
		}
	}
	return result, nil
}
func (m *mockAccountRepoForGemini) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	if m.listByGroupFunc != nil {
		return m.listByGroupFunc(ctx, groupID, platforms)
	}
	return m.ListSchedulableByPlatforms(ctx, platforms)
}
func (m *mockAccountRepoForGemini) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return m.ListSchedulableByPlatform(ctx, platform)
}
func (m *mockAccountRepoForGemini) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	return m.ListSchedulableByPlatforms(ctx, platforms)
}
func (m *mockAccountRepoForGemini) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return nil
}
func (m *mockAccountRepoForGemini) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return nil
}
func (m *mockAccountRepoForGemini) ClearTempUnschedulable(ctx context.Context, id int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) ClearRateLimit(ctx context.Context, id int64) error { return nil }
func (m *mockAccountRepoForGemini) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) ClearModelRateLimits(ctx context.Context, id int64) error {
	return nil
}
func (m *mockAccountRepoForGemini) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	return nil
}
func (m *mockAccountRepoForGemini) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	return nil
}
func (m *mockAccountRepoForGemini) BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	return 0, nil
}

func (m *mockAccountRepoForGemini) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	return nil
}

func (m *mockAccountRepoForGemini) ResetQuotaUsed(ctx context.Context, id int64) error {
	return nil
}

// Verify interface implementation
var _ AccountRepository = (*mockAccountRepoForGemini)(nil)

// mockGroupRepoForGemini Gemini 测试用的 group repo mock
type mockGroupRepoForGemini struct {
	groups           map[int64]*Group
	getByIDCalls     int
	getByIDLiteCalls int
}

func (m *mockGroupRepoForGemini) GetByID(ctx context.Context, id int64) (*Group, error) {
	m.getByIDCalls++
	if g, ok := m.groups[id]; ok {
		return g, nil
	}
	return nil, errors.New("group not found")
}

func (m *mockGroupRepoForGemini) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	m.getByIDLiteCalls++
	if g, ok := m.groups[id]; ok {
		return g, nil
	}
	return nil, errors.New("group not found")
}

// Stub methods to implement GroupRepository interface
func (m *mockGroupRepoForGemini) Create(ctx context.Context, group *Group) error { return nil }
func (m *mockGroupRepoForGemini) Update(ctx context.Context, group *Group) error { return nil }
func (m *mockGroupRepoForGemini) Delete(ctx context.Context, id int64) error     { return nil }
func (m *mockGroupRepoForGemini) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, nil
}
func (m *mockGroupRepoForGemini) List(ctx context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockGroupRepoForGemini) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (m *mockGroupRepoForGemini) ListActive(ctx context.Context) ([]Group, error) { return nil, nil }
func (m *mockGroupRepoForGemini) ListActiveByPlatform(ctx context.Context, platform string) ([]Group, error) {
	return nil, nil
}
func (m *mockGroupRepoForGemini) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, nil
}
func (m *mockGroupRepoForGemini) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	return 0, 0, nil
}
func (m *mockGroupRepoForGemini) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
}

func (m *mockGroupRepoForGemini) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return nil
}

func (m *mockGroupRepoForGemini) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, nil
}

func (m *mockGroupRepoForGemini) UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	return nil
}

var _ GroupRepository = (*mockGroupRepoForGemini)(nil)

// mockGatewayCacheForGemini Gemini 测试用的 cache mock
type mockGatewayCacheForGemini struct {
	sessionBindings map[string]int64
	deletedSessions map[string]int
}

func (m *mockGatewayCacheForGemini) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	if id, ok := m.sessionBindings[sessionHash]; ok {
		return id, nil
	}
	return 0, errors.New("not found")
}

func (m *mockGatewayCacheForGemini) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if m.sessionBindings == nil {
		m.sessionBindings = make(map[string]int64)
	}
	m.sessionBindings[sessionHash] = accountID
	return nil
}

func (m *mockGatewayCacheForGemini) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	return nil
}

func (m *mockGatewayCacheForGemini) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	if m.sessionBindings == nil {
		return nil
	}
	if m.deletedSessions == nil {
		m.deletedSessions = make(map[string]int)
	}
	m.deletedSessions[sessionHash]++
	delete(m.sessionBindings, sessionHash)
	return nil
}

// TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_GeminiPlatform 测试 Gemini 单平台选择
func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_GeminiPlatform(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForGemini{
		accounts: []Account{
			{ID: 1, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true},
			{ID: 2, Platform: PlatformGemini, Priority: 2, Status: StatusActive, Schedulable: true},
			{ID: 3, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: true}, // 应被隔离
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	// 无分组时使用 gemini 平台
	acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "", "gemini-2.5-flash", nil)
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, int64(1), acc.ID, "应选择优先级最高的 gemini 账户")
	require.Equal(t, PlatformGemini, acc.Platform, "无分组时应只返回 gemini 平台账户")
}

func TestGeminiMessagesCompatService_GroupResolution_ReusesContextGroup(t *testing.T) {
	ctx := context.Background()
	groupID := int64(7)
	group := &Group{
		ID:       groupID,
		Platform: PlatformGemini,
		Status:   StatusActive,
		Hydrated: true,
	}
	ctx = context.WithValue(ctx, ctxkey.Group, group)

	repo := &mockAccountRepoForGemini{
		accounts: []Account{
			{ID: 1, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "gemini-2.5-flash", nil)
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, 0, groupRepo.getByIDCalls)
	require.Equal(t, 0, groupRepo.getByIDLiteCalls)
}

func TestGeminiMessagesCompatService_GroupResolution_UsesLiteFetch(t *testing.T) {
	ctx := context.Background()
	groupID := int64(7)

	repo := &mockAccountRepoForGemini{
		accounts: []Account{
			{ID: 1, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{
		groups: map[int64]*Group{
			groupID: {ID: groupID, Platform: PlatformGemini},
		},
	}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "gemini-2.5-flash", nil)
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, 0, groupRepo.getByIDCalls)
	require.Equal(t, 1, groupRepo.getByIDLiteCalls)
}

// TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_AntigravityGroup 测试 antigravity 分组
func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_AntigravityGroup(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForGemini{
		accounts: []Account{
			{ID: 1, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true},      // 应被隔离
			{ID: 2, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: true}, // 应被选择
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{
		groups: map[int64]*Group{
			1: {ID: 1, Platform: PlatformAntigravity},
		},
	}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	groupID := int64(1)
	acc, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "gemini-2.5-flash", nil)
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID)
	require.Equal(t, PlatformAntigravity, acc.Platform, "antigravity 分组应只返回 antigravity 账户")
}

// TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_OAuthPreferred 测试 OAuth 优先
func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_OAuthPreferred(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForGemini{
		accounts: []Account{
			{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Priority: 1, Status: StatusActive, Schedulable: true, LastUsedAt: nil},
			{ID: 2, Platform: PlatformGemini, Type: AccountTypeOAuth, Priority: 1, Status: StatusActive, Schedulable: true, LastUsedAt: nil},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "", "gemini-2.5-flash", nil)
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID, "同优先级且都未使用时，应优先选择 OAuth 账户")
	require.Equal(t, AccountTypeOAuth, acc.Type)
}

// TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_NoAvailableAccounts 测试无可用账户
func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_NoAvailableAccounts(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForGemini{
		accounts:     []Account{},
		accountsByID: map[int64]*Account{},
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "", "gemini-2.5-flash", nil)
	require.Error(t, err)
	require.Nil(t, acc)
	require.Contains(t, err.Error(), "no available")
}

// TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_StickySession 测试粘性会话
func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_StickySession(t *testing.T) {
	ctx := context.Background()

	t.Run("粘性会话命中-同平台", func(t *testing.T) {
		repo := &mockAccountRepoForGemini{
			accounts: []Account{
				{ID: 1, Platform: PlatformGemini, Priority: 2, Status: StatusActive, Schedulable: true},
				{ID: 2, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		// 注意：缓存键使用 "gemini:" 前缀
		cache := &mockGatewayCacheForGemini{
			sessionBindings: map[string]int64{"gemini:session-123": 1},
		}
		groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

		svc := &GeminiMessagesCompatService{
			accountRepo: repo,
			groupRepo:   groupRepo,
			cache:       cache,
		}

		acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "session-123", "gemini-2.5-flash", nil)
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID, "应返回粘性会话绑定的账户")
	})

	t.Run("粘性会话平台不匹配-降级选择", func(t *testing.T) {
		repo := &mockAccountRepoForGemini{
			accounts: []Account{
				{ID: 1, Platform: PlatformAntigravity, Priority: 2, Status: StatusActive, Schedulable: true}, // 粘性会话绑定
				{ID: 2, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		cache := &mockGatewayCacheForGemini{
			sessionBindings: map[string]int64{"gemini:session-123": 1}, // 绑定 antigravity 账户
		}
		groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

		svc := &GeminiMessagesCompatService{
			accountRepo: repo,
			groupRepo:   groupRepo,
			cache:       cache,
		}

		// 无分组时使用 gemini 平台，粘性会话绑定的 antigravity 账户平台不匹配
		acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "session-123", "gemini-2.5-flash", nil)
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "粘性会话账户平台不匹配，应降级选择 gemini 账户")
		require.Equal(t, PlatformGemini, acc.Platform)
	})

	t.Run("粘性会话不命中无前缀缓存键", func(t *testing.T) {
		repo := &mockAccountRepoForGemini{
			accounts: []Account{
				{ID: 1, Platform: PlatformGemini, Priority: 2, Status: StatusActive, Schedulable: true},
				{ID: 2, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		// 缓存键没有 "gemini:" 前缀，不应命中
		cache := &mockGatewayCacheForGemini{
			sessionBindings: map[string]int64{"session-123": 1},
		}
		groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

		svc := &GeminiMessagesCompatService{
			accountRepo: repo,
			groupRepo:   groupRepo,
			cache:       cache,
		}

		acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "session-123", "gemini-2.5-flash", nil)
		require.NoError(t, err)
		require.NotNil(t, acc)
		// 粘性会话未命中，按优先级选择
		require.Equal(t, int64(2), acc.ID, "粘性会话未命中，应按优先级选择")
	})

	t.Run("粘性会话不可调度-清理并回退选择", func(t *testing.T) {
		repo := &mockAccountRepoForGemini{
			accounts: []Account{
				{ID: 1, Platform: PlatformGemini, Priority: 2, Status: StatusDisabled, Schedulable: true},
				{ID: 2, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true},
			},
			accountsByID: map[int64]*Account{},
		}
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		}

		cache := &mockGatewayCacheForGemini{
			sessionBindings: map[string]int64{"gemini:session-123": 1},
		}
		groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

		svc := &GeminiMessagesCompatService{
			accountRepo: repo,
			groupRepo:   groupRepo,
			cache:       cache,
		}

		acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "session-123", "gemini-2.5-flash", nil)
		require.NoError(t, err)
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID)
		require.Equal(t, 1, cache.deletedSessions["gemini:session-123"])
		require.Equal(t, int64(2), cache.sessionBindings["gemini:session-123"])
	})
}

func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_ForcePlatformFallback(t *testing.T) {
	ctx := context.Background()
	groupID := int64(9)
	ctx = context.WithValue(ctx, ctxkey.ForcePlatform, PlatformAntigravity)

	repo := &mockAccountRepoForGemini{
		listByGroupFunc: func(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
			return nil, nil
		},
		listByPlatformFunc: func(ctx context.Context, platforms []string) ([]Account, error) {
			return []Account{
				{ID: 1, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: true},
			}, nil
		},
		accountsByID: map[int64]*Account{
			1: {ID: 1, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: true},
		},
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "gemini-2.5-flash", nil)
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, int64(1), acc.ID)
}

func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_NoModelSupport(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForGemini{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformGemini,
				Priority:    1,
				Status:      StatusActive,
				Schedulable: true,
				Credentials: map[string]any{"model_mapping": map[string]any{"gemini-1.0-pro": "gemini-1.0-pro"}},
			},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "", "gemini-2.5-flash", nil)
	require.Error(t, err)
	require.Nil(t, acc)
	require.Contains(t, err.Error(), "supporting model")
}

func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_StickyMixedScheduling(t *testing.T) {
	ctx := context.Background()
	repo := &mockAccountRepoForGemini{
		accounts: []Account{
			{ID: 1, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: true, Extra: map[string]any{"mixed_scheduling": true}},
			{ID: 2, Platform: PlatformGemini, Priority: 2, Status: StatusActive, Schedulable: true},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cache := &mockGatewayCacheForGemini{
		sessionBindings: map[string]int64{"gemini:session-999": 1},
	}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "session-999", "gemini-2.5-flash", nil)
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, int64(1), acc.ID)
}

func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_SkipDisabledMixedScheduling(t *testing.T) {
	ctx := context.Background()
	repo := &mockAccountRepoForGemini{
		accounts: []Account{
			{ID: 1, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: true},
			{ID: 2, Platform: PlatformGemini, Priority: 2, Status: StatusActive, Schedulable: true},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "", "gemini-2.5-flash", nil)
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID)
}

func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_ExcludedAccount(t *testing.T) {
	ctx := context.Background()
	repo := &mockAccountRepoForGemini{
		accounts: []Account{
			{ID: 1, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true},
			{ID: 2, Platform: PlatformGemini, Priority: 2, Status: StatusActive, Schedulable: true},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	excluded := map[int64]struct{}{1: {}}
	acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "", "gemini-2.5-flash", excluded)
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID)
}

func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_ListError(t *testing.T) {
	ctx := context.Background()
	repo := &mockAccountRepoForGemini{
		listByPlatformFunc: func(ctx context.Context, platforms []string) ([]Account, error) {
			return nil, errors.New("query failed")
		},
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "", "gemini-2.5-flash", nil)
	require.Error(t, err)
	require.Nil(t, acc)
	require.Contains(t, err.Error(), "query accounts failed")
}

func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_PreferOAuth(t *testing.T) {
	ctx := context.Background()
	repo := &mockAccountRepoForGemini{
		accounts: []Account{
			{ID: 1, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKey},
			{ID: 2, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuth},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "", "gemini-2.5-pro", nil)
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID)
}

func TestGeminiMessagesCompatService_SelectAccountForModelWithExclusions_PreferLeastRecentlyUsed(t *testing.T) {
	ctx := context.Background()
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	repo := &mockAccountRepoForGemini{
		accounts: []Account{
			{ID: 1, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true, LastUsedAt: &newTime},
			{ID: 2, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true, LastUsedAt: &oldTime},
		},
		accountsByID: map[int64]*Account{},
	}
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	}

	cache := &mockGatewayCacheForGemini{}
	groupRepo := &mockGroupRepoForGemini{groups: map[int64]*Group{}}

	svc := &GeminiMessagesCompatService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cache:       cache,
	}

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "", "gemini-2.5-pro", nil)
	require.NoError(t, err)
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID)
}

// TestGeminiPlatformRouting_DocumentRouteDecision 测试平台路由决策逻辑
func TestGeminiPlatformRouting_DocumentRouteDecision(t *testing.T) {
	tests := []struct {
		name            string
		platform        string
		expectedService string // "gemini" 表示 ForwardNative, "antigravity" 表示 ForwardGemini
	}{
		{
			name:            "Gemini平台走ForwardNative",
			platform:        PlatformGemini,
			expectedService: "gemini",
		},
		{
			name:            "Antigravity平台走ForwardGemini",
			platform:        PlatformAntigravity,
			expectedService: "antigravity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Platform: tt.platform}

			// 模拟 Handler 层的路由逻辑
			var serviceName string
			if account.Platform == PlatformAntigravity {
				serviceName = "antigravity"
			} else {
				serviceName = "gemini"
			}

			require.Equal(t, tt.expectedService, serviceName,
				"平台 %s 应该路由到 %s 服务", tt.platform, tt.expectedService)
		})
	}
}

func TestGeminiMessagesCompatService_isModelSupportedByAccount(t *testing.T) {
	svc := &GeminiMessagesCompatService{}

	tests := []struct {
		name     string
		account  *Account
		model    string
		expected bool
	}{
		{
			name:     "Antigravity平台-支持gemini模型",
			account:  &Account{Platform: PlatformAntigravity},
			model:    "gemini-2.5-flash",
			expected: true,
		},
		{
			name:     "Antigravity平台-支持claude模型",
			account:  &Account{Platform: PlatformAntigravity},
			model:    "claude-sonnet-4-5",
			expected: true,
		},
		{
			name:     "Antigravity平台-不支持gpt模型",
			account:  &Account{Platform: PlatformAntigravity},
			model:    "gpt-4",
			expected: false,
		},
		{
			name:     "Antigravity平台-空模型允许",
			account:  &Account{Platform: PlatformAntigravity},
			model:    "",
			expected: true,
		},
		{
			name: "Antigravity平台-自定义映射-支持自定义模型",
			account: &Account{
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"my-custom-model": "upstream-model",
						"gpt-4o":          "some-model",
					},
				},
			},
			model:    "my-custom-model",
			expected: true,
		},
		{
			name: "Antigravity平台-自定义映射-不在映射中的模型不支持",
			account: &Account{
				Platform: PlatformAntigravity,
				Credentials: map[string]any{
					"model_mapping": map[string]any{
						"my-custom-model": "upstream-model",
					},
				},
			},
			model:    "claude-sonnet-4-5",
			expected: false,
		},
		{
			name:     "Gemini平台-无映射配置-支持所有模型",
			account:  &Account{Platform: PlatformGemini},
			model:    "gemini-2.5-flash",
			expected: true,
		},
		{
			name: "Gemini平台-有映射配置-只支持配置的模型",
			account: &Account{
				Platform:    PlatformGemini,
				Credentials: map[string]any{"model_mapping": map[string]any{"gemini-2.5-pro": "x"}},
			},
			model:    "gemini-2.5-flash",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.isModelSupportedByAccount(tt.account, tt.model)
			require.Equal(t, tt.expected, got)
		})
	}
}
