//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func ptrString[T ~string](v T) *string {
	s := string(v)
	return &s
}

// groupRepoStubForAdmin 用于测试 AdminService 的 GroupRepository Stub
type groupRepoStubForAdmin struct {
	created *Group // 记录 Create 调用的参数
	updated *Group // 记录 Update 调用的参数
	getByID *Group // GetByID 返回值
	getErr  error  // GetByID 返回的错误

	listWithFiltersCalls       int
	listWithFiltersParams      pagination.PaginationParams
	listWithFiltersPlatform    string
	listWithFiltersStatus      string
	listWithFiltersSearch      string
	listWithFiltersIsExclusive *bool
	listWithFiltersGroups      []Group
	listWithFiltersResult      *pagination.PaginationResult
	listWithFiltersErr         error
}

func (s *groupRepoStubForAdmin) Create(_ context.Context, g *Group) error {
	s.created = g
	return nil
}

func (s *groupRepoStubForAdmin) Update(_ context.Context, g *Group) error {
	s.updated = g
	return nil
}

func (s *groupRepoStubForAdmin) GetByID(_ context.Context, _ int64) (*Group, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getByID, nil
}

func (s *groupRepoStubForAdmin) GetByIDLite(_ context.Context, _ int64) (*Group, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getByID, nil
}

func (s *groupRepoStubForAdmin) Delete(_ context.Context, _ int64) error {
	panic("unexpected Delete call")
}

func (s *groupRepoStubForAdmin) DeleteCascade(_ context.Context, _ int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
}

func (s *groupRepoStubForAdmin) List(_ context.Context, _ pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *groupRepoStubForAdmin) ListWithFilters(_ context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	s.listWithFiltersCalls++
	s.listWithFiltersParams = params
	s.listWithFiltersPlatform = platform
	s.listWithFiltersStatus = status
	s.listWithFiltersSearch = search
	s.listWithFiltersIsExclusive = isExclusive

	if s.listWithFiltersErr != nil {
		return nil, nil, s.listWithFiltersErr
	}

	result := s.listWithFiltersResult
	if result == nil {
		result = &pagination.PaginationResult{
			Total:    int64(len(s.listWithFiltersGroups)),
			Page:     params.Page,
			PageSize: params.PageSize,
		}
	}

	return s.listWithFiltersGroups, result, nil
}

func (s *groupRepoStubForAdmin) ListActive(_ context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
}

func (s *groupRepoStubForAdmin) ListActiveByPlatform(_ context.Context, _ string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}

func (s *groupRepoStubForAdmin) ExistsByName(_ context.Context, _ string) (bool, error) {
	panic("unexpected ExistsByName call")
}

func (s *groupRepoStubForAdmin) GetAccountCount(_ context.Context, _ int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}

func (s *groupRepoStubForAdmin) DeleteAccountGroupsByGroupID(_ context.Context, _ int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}

func (s *groupRepoStubForAdmin) BindAccountsToGroup(_ context.Context, _ int64, _ []int64) error {
	panic("unexpected BindAccountsToGroup call")
}

func (s *groupRepoStubForAdmin) GetAccountIDsByGroupIDs(_ context.Context, _ []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}

func (s *groupRepoStubForAdmin) UpdateSortOrders(_ context.Context, _ []GroupSortOrderUpdate) error {
	return nil
}

func TestAdminService_ListGroups_PassesSortParams(t *testing.T) {
	repo := &groupRepoStubForAdmin{
		listWithFiltersGroups: []Group{{ID: 1, Name: "g1"}},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, _, err := svc.ListGroups(context.Background(), 3, 25, PlatformOpenAI, StatusActive, "needle", nil, "account_count", "ASC")
	require.NoError(t, err)
	require.Equal(t, pagination.PaginationParams{
		Page:      3,
		PageSize:  25,
		SortBy:    "account_count",
		SortOrder: "ASC",
	}, repo.listWithFiltersParams)
}

// TestAdminService_CreateGroup_WithImagePricing 测试创建分组时 ImagePrice 字段正确传递
func TestAdminService_CreateGroup_WithImagePricing(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	price1K := 0.10
	price2K := 0.15
	price4K := 0.30

	input := &CreateGroupInput{
		Name:           "test-group",
		Description:    "Test group",
		Platform:       PlatformAntigravity,
		RateMultiplier: 1.0,
		ImagePrice1K:   &price1K,
		ImagePrice2K:   &price2K,
		ImagePrice4K:   &price4K,
	}

	group, err := svc.CreateGroup(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, group)

	// 验证 repo 收到了正确的字段
	require.NotNil(t, repo.created)
	require.NotNil(t, repo.created.ImagePrice1K)
	require.NotNil(t, repo.created.ImagePrice2K)
	require.NotNil(t, repo.created.ImagePrice4K)
	require.InDelta(t, 0.10, *repo.created.ImagePrice1K, 0.0001)
	require.InDelta(t, 0.15, *repo.created.ImagePrice2K, 0.0001)
	require.InDelta(t, 0.30, *repo.created.ImagePrice4K, 0.0001)
}

// TestAdminService_CreateGroup_NilImagePricing 测试 ImagePrice 为 nil 时正常创建
func TestAdminService_CreateGroup_NilImagePricing(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	input := &CreateGroupInput{
		Name:           "test-group",
		Description:    "Test group",
		Platform:       PlatformAntigravity,
		RateMultiplier: 1.0,
		// ImagePrice 字段全部为 nil
	}

	group, err := svc.CreateGroup(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, group)

	// 验证 ImagePrice 字段为 nil
	require.NotNil(t, repo.created)
	require.Nil(t, repo.created.ImagePrice1K)
	require.Nil(t, repo.created.ImagePrice2K)
	require.Nil(t, repo.created.ImagePrice4K)
}

// TestAdminService_UpdateGroup_WithImagePricing 测试更新分组时 ImagePrice 字段正确更新
func TestAdminService_UpdateGroup_WithImagePricing(t *testing.T) {
	existingGroup := &Group{
		ID:       1,
		Name:     "existing-group",
		Platform: PlatformAntigravity,
		Status:   StatusActive,
	}
	repo := &groupRepoStubForAdmin{getByID: existingGroup}
	svc := &adminServiceImpl{groupRepo: repo}

	price1K := 0.12
	price2K := 0.18
	price4K := 0.36

	input := &UpdateGroupInput{
		ImagePrice1K: &price1K,
		ImagePrice2K: &price2K,
		ImagePrice4K: &price4K,
	}

	group, err := svc.UpdateGroup(context.Background(), 1, input)
	require.NoError(t, err)
	require.NotNil(t, group)

	// 验证 repo 收到了更新后的字段
	require.NotNil(t, repo.updated)
	require.NotNil(t, repo.updated.ImagePrice1K)
	require.NotNil(t, repo.updated.ImagePrice2K)
	require.NotNil(t, repo.updated.ImagePrice4K)
	require.InDelta(t, 0.12, *repo.updated.ImagePrice1K, 0.0001)
	require.InDelta(t, 0.18, *repo.updated.ImagePrice2K, 0.0001)
	require.InDelta(t, 0.36, *repo.updated.ImagePrice4K, 0.0001)
}

// TestAdminService_UpdateGroup_PartialImagePricing 测试仅更新部分 ImagePrice 字段
func TestAdminService_UpdateGroup_PartialImagePricing(t *testing.T) {
	oldPrice2K := 0.15
	existingGroup := &Group{
		ID:           1,
		Name:         "existing-group",
		Platform:     PlatformAntigravity,
		Status:       StatusActive,
		ImagePrice2K: &oldPrice2K, // 已有 2K 价格
	}
	repo := &groupRepoStubForAdmin{getByID: existingGroup}
	svc := &adminServiceImpl{groupRepo: repo}

	// 只更新 1K 价格
	price1K := 0.10
	input := &UpdateGroupInput{
		ImagePrice1K: &price1K,
		// ImagePrice2K 和 ImagePrice4K 为 nil，不更新
	}

	group, err := svc.UpdateGroup(context.Background(), 1, input)
	require.NoError(t, err)
	require.NotNil(t, group)

	// 验证：1K 被更新，2K 保持原值，4K 仍为 nil
	require.NotNil(t, repo.updated)
	require.NotNil(t, repo.updated.ImagePrice1K)
	require.InDelta(t, 0.10, *repo.updated.ImagePrice1K, 0.0001)
	require.NotNil(t, repo.updated.ImagePrice2K)
	require.InDelta(t, 0.15, *repo.updated.ImagePrice2K, 0.0001) // 原值保持
	require.Nil(t, repo.updated.ImagePrice4K)
}

func TestAdminService_CreateGroup_NormalizesMessagesDispatchModelConfig(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:           "dispatch-group",
		Description:    "dispatch config",
		Platform:       PlatformOpenAI,
		RateMultiplier: 1.0,
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			OpusMappedModel:   " gpt-5.4-high ",
			SonnetMappedModel: " gpt-5.3-codex ",
			HaikuMappedModel:  " gpt-5.4-mini-medium ",
			ExactModelMappings: map[string]string{
				" claude-sonnet-4-5-20250929 ": " gpt-5.2-high ",
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.Equal(t, OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:   "gpt-5.4",
		SonnetMappedModel: "gpt-5.3-codex",
		HaikuMappedModel:  "gpt-5.4-mini",
		ExactModelMappings: map[string]string{
			"claude-sonnet-4-5-20250929": "gpt-5.2",
		},
	}, repo.created.MessagesDispatchModelConfig)
}

func TestAdminService_UpdateGroup_NormalizesMessagesDispatchModelConfig(t *testing.T) {
	existingGroup := &Group{
		ID:       1,
		Name:     "existing-group",
		Platform: PlatformOpenAI,
		Status:   StatusActive,
	}
	repo := &groupRepoStubForAdmin{getByID: existingGroup}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		MessagesDispatchModelConfig: &OpenAIMessagesDispatchModelConfig{
			SonnetMappedModel: " gpt-5.4-medium ",
			ExactModelMappings: map[string]string{
				" claude-haiku-4-5-20251001 ": " gpt-5.4-mini-high ",
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Equal(t, OpenAIMessagesDispatchModelConfig{
		SonnetMappedModel: "gpt-5.4",
		ExactModelMappings: map[string]string{
			"claude-haiku-4-5-20251001": "gpt-5.4-mini",
		},
	}, repo.updated.MessagesDispatchModelConfig)
}

func TestAdminService_CreateGroup_ClearsMessagesDispatchFieldsForNonOpenAIPlatform(t *testing.T) {
	repo := &groupRepoStubForAdmin{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                  "anthropic-group",
		Description:           "non-openai",
		Platform:              PlatformAnthropic,
		RateMultiplier:        1.0,
		AllowMessagesDispatch: true,
		DefaultMappedModel:    "gpt-5.4",
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			OpusMappedModel: "gpt-5.4",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.False(t, repo.created.AllowMessagesDispatch)
	require.Empty(t, repo.created.DefaultMappedModel)
	require.Equal(t, OpenAIMessagesDispatchModelConfig{}, repo.created.MessagesDispatchModelConfig)
}

func TestAdminService_UpdateGroup_ClearsMessagesDispatchFieldsWhenPlatformChangesAwayFromOpenAI(t *testing.T) {
	existingGroup := &Group{
		ID:                    1,
		Name:                  "existing-openai-group",
		Platform:              PlatformOpenAI,
		Status:                StatusActive,
		AllowMessagesDispatch: true,
		DefaultMappedModel:    "gpt-5.4",
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			SonnetMappedModel: "gpt-5.3-codex",
		},
	}
	repo := &groupRepoStubForAdmin{getByID: existingGroup}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		Platform: PlatformAnthropic,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Equal(t, PlatformAnthropic, repo.updated.Platform)
	require.False(t, repo.updated.AllowMessagesDispatch)
	require.Empty(t, repo.updated.DefaultMappedModel)
	require.Equal(t, OpenAIMessagesDispatchModelConfig{}, repo.updated.MessagesDispatchModelConfig)
}

func TestAdminService_ListGroups_WithSearch(t *testing.T) {
	// 测试：
	// 1. search 参数正常传递到 repository 层
	// 2. search 为空字符串时的行为
	// 3. search 与其他过滤条件组合使用

	t.Run("search 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{
			listWithFiltersGroups: []Group{{ID: 1, Name: "alpha"}},
			listWithFiltersResult: &pagination.PaginationResult{Total: 1},
		}
		svc := &adminServiceImpl{groupRepo: repo}

		groups, total, err := svc.ListGroups(context.Background(), 1, 20, "", "", "alpha", nil, "", "")
		require.NoError(t, err)
		require.Equal(t, int64(1), total)
		require.Equal(t, []Group{{ID: 1, Name: "alpha"}}, groups)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 1, PageSize: 20}, repo.listWithFiltersParams)
		require.Equal(t, "alpha", repo.listWithFiltersSearch)
		require.Nil(t, repo.listWithFiltersIsExclusive)
	})

	t.Run("search 为空字符串时传递空字符串", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{
			listWithFiltersGroups: []Group{},
			listWithFiltersResult: &pagination.PaginationResult{Total: 0},
		}
		svc := &adminServiceImpl{groupRepo: repo}

		groups, total, err := svc.ListGroups(context.Background(), 2, 10, "", "", "", nil, "", "")
		require.NoError(t, err)
		require.Empty(t, groups)
		require.Equal(t, int64(0), total)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 2, PageSize: 10}, repo.listWithFiltersParams)
		require.Equal(t, "", repo.listWithFiltersSearch)
		require.Nil(t, repo.listWithFiltersIsExclusive)
	})

	t.Run("search 与其他过滤条件组合使用", func(t *testing.T) {
		isExclusive := true
		repo := &groupRepoStubForAdmin{
			listWithFiltersGroups: []Group{{ID: 2, Name: "beta"}},
			listWithFiltersResult: &pagination.PaginationResult{Total: 42},
		}
		svc := &adminServiceImpl{groupRepo: repo}

		groups, total, err := svc.ListGroups(context.Background(), 3, 50, PlatformAntigravity, StatusActive, "beta", &isExclusive, "", "")
		require.NoError(t, err)
		require.Equal(t, int64(42), total)
		require.Equal(t, []Group{{ID: 2, Name: "beta"}}, groups)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 3, PageSize: 50}, repo.listWithFiltersParams)
		require.Equal(t, PlatformAntigravity, repo.listWithFiltersPlatform)
		require.Equal(t, StatusActive, repo.listWithFiltersStatus)
		require.Equal(t, "beta", repo.listWithFiltersSearch)
		require.NotNil(t, repo.listWithFiltersIsExclusive)
		require.True(t, *repo.listWithFiltersIsExclusive)
	})
}

func TestAdminService_ValidateFallbackGroup_DetectsCycle(t *testing.T) {
	groupID := int64(1)
	fallbackID := int64(2)
	repo := &groupRepoStubForFallbackCycle{
		groups: map[int64]*Group{
			groupID: {
				ID:              groupID,
				FallbackGroupID: &fallbackID,
			},
			fallbackID: {
				ID:              fallbackID,
				FallbackGroupID: &groupID,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	err := svc.validateFallbackGroup(context.Background(), groupID, fallbackID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group cycle")
}

type groupRepoStubForFallbackCycle struct {
	groups map[int64]*Group
}

func (s *groupRepoStubForFallbackCycle) Create(_ context.Context, _ *Group) error {
	panic("unexpected Create call")
}

func (s *groupRepoStubForFallbackCycle) Update(_ context.Context, _ *Group) error {
	panic("unexpected Update call")
}

func (s *groupRepoStubForFallbackCycle) GetByID(ctx context.Context, id int64) (*Group, error) {
	return s.GetByIDLite(ctx, id)
}

func (s *groupRepoStubForFallbackCycle) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	if g, ok := s.groups[id]; ok {
		return g, nil
	}
	return nil, ErrGroupNotFound
}

func (s *groupRepoStubForFallbackCycle) Delete(_ context.Context, _ int64) error {
	panic("unexpected Delete call")
}

func (s *groupRepoStubForFallbackCycle) DeleteCascade(_ context.Context, _ int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
}

func (s *groupRepoStubForFallbackCycle) List(_ context.Context, _ pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *groupRepoStubForFallbackCycle) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _, _ string, _ *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *groupRepoStubForFallbackCycle) ListActive(_ context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
}

func (s *groupRepoStubForFallbackCycle) ListActiveByPlatform(_ context.Context, _ string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}

func (s *groupRepoStubForFallbackCycle) ExistsByName(_ context.Context, _ string) (bool, error) {
	panic("unexpected ExistsByName call")
}

func (s *groupRepoStubForFallbackCycle) GetAccountCount(_ context.Context, _ int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}

func (s *groupRepoStubForFallbackCycle) DeleteAccountGroupsByGroupID(_ context.Context, _ int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}

func (s *groupRepoStubForFallbackCycle) BindAccountsToGroup(_ context.Context, _ int64, _ []int64) error {
	panic("unexpected BindAccountsToGroup call")
}

func (s *groupRepoStubForFallbackCycle) GetAccountIDsByGroupIDs(_ context.Context, _ []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}

func (s *groupRepoStubForFallbackCycle) UpdateSortOrders(_ context.Context, _ []GroupSortOrderUpdate) error {
	return nil
}

type groupRepoStubForInvalidRequestFallback struct {
	groups  map[int64]*Group
	created *Group
	updated *Group
}

func (s *groupRepoStubForInvalidRequestFallback) Create(_ context.Context, g *Group) error {
	s.created = g
	return nil
}

func (s *groupRepoStubForInvalidRequestFallback) Update(_ context.Context, g *Group) error {
	s.updated = g
	return nil
}

func (s *groupRepoStubForInvalidRequestFallback) GetByID(ctx context.Context, id int64) (*Group, error) {
	return s.GetByIDLite(ctx, id)
}

func (s *groupRepoStubForInvalidRequestFallback) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	if g, ok := s.groups[id]; ok {
		return g, nil
	}
	return nil, ErrGroupNotFound
}

func (s *groupRepoStubForInvalidRequestFallback) Delete(_ context.Context, _ int64) error {
	panic("unexpected Delete call")
}

func (s *groupRepoStubForInvalidRequestFallback) DeleteCascade(_ context.Context, _ int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
}

func (s *groupRepoStubForInvalidRequestFallback) List(_ context.Context, _ pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *groupRepoStubForInvalidRequestFallback) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _, _ string, _ *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *groupRepoStubForInvalidRequestFallback) ListActive(_ context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
}

func (s *groupRepoStubForInvalidRequestFallback) ListActiveByPlatform(_ context.Context, _ string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}

func (s *groupRepoStubForInvalidRequestFallback) ExistsByName(_ context.Context, _ string) (bool, error) {
	panic("unexpected ExistsByName call")
}

func (s *groupRepoStubForInvalidRequestFallback) GetAccountCount(_ context.Context, _ int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}

func (s *groupRepoStubForInvalidRequestFallback) DeleteAccountGroupsByGroupID(_ context.Context, _ int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}

func (s *groupRepoStubForInvalidRequestFallback) GetAccountIDsByGroupIDs(_ context.Context, _ []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}

func (s *groupRepoStubForInvalidRequestFallback) BindAccountsToGroup(_ context.Context, _ int64, _ []int64) error {
	panic("unexpected BindAccountsToGroup call")
}

func (s *groupRepoStubForInvalidRequestFallback) UpdateSortOrders(_ context.Context, _ []GroupSortOrderUpdate) error {
	return nil
}

func TestAdminService_CreateGroup_InvalidRequestFallbackRejectsUnsupportedPlatform(t *testing.T) {
	fallbackID := int64(10)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                            "g1",
		Platform:                        PlatformOpenAI,
		SubscriptionType:                SubscriptionTypeStandard,
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid request fallback only supported for anthropic or antigravity groups")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_InvalidRequestFallbackRejectsSubscription(t *testing.T) {
	fallbackID := int64(10)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                            "g1",
		Platform:                        PlatformAnthropic,
		SubscriptionType:                SubscriptionTypeSubscription,
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription groups cannot set invalid request fallback")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_InvalidRequestFallbackRejectsFallbackGroup(t *testing.T) {
	tests := []struct {
		name        string
		fallback    *Group
		wantMessage string
	}{
		{
			name:        "openai_target",
			fallback:    &Group{ID: 10, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard},
			wantMessage: "fallback group must be anthropic platform",
		},
		{
			name:        "antigravity_target",
			fallback:    &Group{ID: 10, Platform: PlatformAntigravity, SubscriptionType: SubscriptionTypeStandard},
			wantMessage: "fallback group must be anthropic platform",
		},
		{
			name:        "subscription_group",
			fallback:    &Group{ID: 10, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeSubscription},
			wantMessage: "fallback group cannot be subscription type",
		},
		{
			name: "nested_fallback",
			fallback: &Group{
				ID:                              10,
				Platform:                        PlatformAnthropic,
				SubscriptionType:                SubscriptionTypeStandard,
				FallbackGroupIDOnInvalidRequest: func() *int64 { v := int64(99); return &v }(),
			},
			wantMessage: "fallback group cannot have invalid request fallback configured",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fallbackID := tc.fallback.ID
			repo := &groupRepoStubForInvalidRequestFallback{
				groups: map[int64]*Group{
					fallbackID: tc.fallback,
				},
			}
			svc := &adminServiceImpl{groupRepo: repo}

			_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
				Name:                            "g1",
				Platform:                        PlatformAnthropic,
				SubscriptionType:                SubscriptionTypeStandard,
				FallbackGroupIDOnInvalidRequest: &fallbackID,
			})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantMessage)
			require.Nil(t, repo.created)
		})
	}
}

func TestAdminService_CreateGroup_InvalidRequestFallbackNotFound(t *testing.T) {
	fallbackID := int64(10)
	repo := &groupRepoStubForInvalidRequestFallback{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                            "g1",
		Platform:                        PlatformAnthropic,
		SubscriptionType:                SubscriptionTypeStandard,
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group not found")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_InvalidRequestFallbackAllowsAntigravity(t *testing.T) {
	fallbackID := int64(10)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                            "g1",
		Platform:                        PlatformAntigravity,
		SubscriptionType:                SubscriptionTypeStandard,
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.Equal(t, fallbackID, *repo.created.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_CreateGroup_InvalidRequestFallbackClearsOnZero(t *testing.T) {
	zero := int64(0)
	repo := &groupRepoStubForInvalidRequestFallback{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                            "g1",
		Platform:                        PlatformAnthropic,
		SubscriptionType:                SubscriptionTypeStandard,
		FallbackGroupIDOnInvalidRequest: &zero,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.Nil(t, repo.created.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackPlatformMismatch(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:                              1,
		Name:                            "g1",
		Platform:                        PlatformAnthropic,
		SubscriptionType:                SubscriptionTypeStandard,
		Status:                          StatusActive,
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID:  {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		Platform: PlatformOpenAI,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid request fallback only supported for anthropic or antigravity groups")
	require.Nil(t, repo.updated)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackSubscriptionMismatch(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:                              1,
		Name:                            "g1",
		Platform:                        PlatformAnthropic,
		SubscriptionType:                SubscriptionTypeStandard,
		Status:                          StatusActive,
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID:  {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		SubscriptionType: SubscriptionTypeSubscription,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription groups cannot set invalid request fallback")
	require.Nil(t, repo.updated)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackClearsOnZero(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:                              1,
		Name:                            "g1",
		Platform:                        PlatformAnthropic,
		SubscriptionType:                SubscriptionTypeStandard,
		Status:                          StatusActive,
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID:  {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	clear := int64(0)
	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		Platform:                        PlatformOpenAI,
		FallbackGroupIDOnInvalidRequest: &clear,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Nil(t, repo.updated.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackRejectsFallbackGroup(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:               1,
		Name:             "g1",
		Platform:         PlatformAnthropic,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID:  {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeSubscription},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group cannot be subscription type")
	require.Nil(t, repo.updated)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackSetSuccess(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:               1,
		Name:             "g1",
		Platform:         PlatformAnthropic,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID:  {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Equal(t, fallbackID, *repo.updated.FallbackGroupIDOnInvalidRequest)
}

func TestAdminService_UpdateGroup_InvalidRequestFallbackAllowsAntigravity(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:               1,
		Name:             "g1",
		Platform:         PlatformAntigravity,
		SubscriptionType: SubscriptionTypeStandard,
		Status:           StatusActive,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID:  {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.updated)
	require.Equal(t, fallbackID, *repo.updated.FallbackGroupIDOnInvalidRequest)
}
