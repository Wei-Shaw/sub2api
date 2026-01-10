//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

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
REDACTED

func (s *groupRepoStubForAdmin) Create(_ context.Context, g *Group) error {
	s.created = g
	return nil
REDACTED

func (s *groupRepoStubForAdmin) Update(_ context.Context, g *Group) error {
	s.updated = g
	return nil
REDACTED

func (s *groupRepoStubForAdmin) GetByID(_ context.Context, _ int64) (*Group, error) {
	if s.getErr != nil {
		return nil, s.getErr
REDACTED
	return s.getByID, nil
REDACTED

func (s *groupRepoStubForAdmin) GetByIDLite(_ context.Context, _ int64) (*Group, error) {
	if s.getErr != nil {
		return nil, s.getErr
REDACTED
	return s.getByID, nil
REDACTED

func (s *groupRepoStubForAdmin) Delete(_ context.Context, _ int64) error {
	panic("unexpected Delete call")
REDACTED

func (s *groupRepoStubForAdmin) DeleteCascade(_ context.Context, _ int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
REDACTED

func (s *groupRepoStubForAdmin) List(_ context.Context, _ pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
REDACTED

func (s *groupRepoStubForAdmin) ListWithFilters(_ context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	s.listWithFiltersCalls++
	s.listWithFiltersParams = params
	s.listWithFiltersPlatform = platform
	s.listWithFiltersStatus = status
	s.listWithFiltersSearch = search
	s.listWithFiltersIsExclusive = isExclusive

	if s.listWithFiltersErr != nil {
		return nil, nil, s.listWithFiltersErr
REDACTED

	result := s.listWithFiltersResult
	if result == nil {
		result = &pagination.PaginationResult{
			Total:    int64(len(s.listWithFiltersGroups)),
			Page:     params.Page,
			PageSize: params.PageSize,
	REDACTED
REDACTED

	return s.listWithFiltersGroups, result, nil
REDACTED

func (s *groupRepoStubForAdmin) ListActive(_ context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
REDACTED

func (s *groupRepoStubForAdmin) ListActiveByPlatform(_ context.Context, _ string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
REDACTED

func (s *groupRepoStubForAdmin) ExistsByName(_ context.Context, _ string) (bool, error) {
	panic("unexpected ExistsByName call")
REDACTED

func (s *groupRepoStubForAdmin) GetAccountCount(_ context.Context, _ int64) (int64, error) {
	panic("unexpected GetAccountCount call")
REDACTED

func (s *groupRepoStubForAdmin) DeleteAccountGroupsByGroupID(_ context.Context, _ int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
REDACTED

// TestAdminService_CreateGroup_WithImagePricing 测试创建分组时 ImagePrice 字段正确传递
func TestAdminService_CreateGroup_WithImagePricing(t *testing.T) {
	repo := &groupRepoStubForAdmin{REDACTED
	svc := &adminServiceImpl{groupRepo: repoREDACTED

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
REDACTED

	group, err := svc.CreateGroup(context.Background(), input)
REDACTED
	require.NotNil(t, group)

	// 验证 repo 收到了正确的字段
	require.NotNil(t, repo.created)
	require.NotNil(t, repo.created.ImagePrice1K)
	require.NotNil(t, repo.created.ImagePrice2K)
	require.NotNil(t, repo.created.ImagePrice4K)
	require.InDelta(t, 0.10, *repo.created.ImagePrice1K, 0.0001)
	require.InDelta(t, 0.15, *repo.created.ImagePrice2K, 0.0001)
	require.InDelta(t, 0.30, *repo.created.ImagePrice4K, 0.0001)
REDACTED

// TestAdminService_CreateGroup_NilImagePricing 测试 ImagePrice 为 nil 时正常创建
func TestAdminService_CreateGroup_NilImagePricing(t *testing.T) {
	repo := &groupRepoStubForAdmin{REDACTED
	svc := &adminServiceImpl{groupRepo: repoREDACTED

	input := &CreateGroupInput{
		Name:           "test-group",
		Description:    "Test group",
		Platform:       PlatformAntigravity,
		RateMultiplier: 1.0,
		// ImagePrice 字段全部为 nil
REDACTED

	group, err := svc.CreateGroup(context.Background(), input)
REDACTED
	require.NotNil(t, group)

	// 验证 ImagePrice 字段为 nil
	require.NotNil(t, repo.created)
	require.Nil(t, repo.created.ImagePrice1K)
	require.Nil(t, repo.created.ImagePrice2K)
	require.Nil(t, repo.created.ImagePrice4K)
REDACTED

// TestAdminService_UpdateGroup_WithImagePricing 测试更新分组时 ImagePrice 字段正确更新
func TestAdminService_UpdateGroup_WithImagePricing(t *testing.T) {
	existingGroup := &Group{
		ID:       1,
		Name:     "existing-group",
		Platform: PlatformAntigravity,
		Status:   StatusActive,
REDACTED
	repo := &groupRepoStubForAdmin{getByID: existingGroupREDACTED
	svc := &adminServiceImpl{groupRepo: repoREDACTED

	price1K := 0.12
	price2K := 0.18
	price4K := 0.36

	input := &UpdateGroupInput{
		ImagePrice1K: &price1K,
		ImagePrice2K: &price2K,
		ImagePrice4K: &price4K,
REDACTED

	group, err := svc.UpdateGroup(context.Background(), 1, input)
REDACTED
	require.NotNil(t, group)

	// 验证 repo 收到了更新后的字段
	require.NotNil(t, repo.updated)
	require.NotNil(t, repo.updated.ImagePrice1K)
	require.NotNil(t, repo.updated.ImagePrice2K)
	require.NotNil(t, repo.updated.ImagePrice4K)
	require.InDelta(t, 0.12, *repo.updated.ImagePrice1K, 0.0001)
	require.InDelta(t, 0.18, *repo.updated.ImagePrice2K, 0.0001)
	require.InDelta(t, 0.36, *repo.updated.ImagePrice4K, 0.0001)
REDACTED

// TestAdminService_UpdateGroup_PartialImagePricing 测试仅更新部分 ImagePrice 字段
func TestAdminService_UpdateGroup_PartialImagePricing(t *testing.T) {
	oldPrice2K := 0.15
	existingGroup := &Group{
		ID:           1,
		Name:         "existing-group",
		Platform:     PlatformAntigravity,
		Status:       StatusActive,
		ImagePrice2K: &oldPrice2K, // 已有 2K 价格
REDACTED
	repo := &groupRepoStubForAdmin{getByID: existingGroupREDACTED
	svc := &adminServiceImpl{groupRepo: repoREDACTED

	// 只更新 1K 价格
	price1K := 0.10
	input := &UpdateGroupInput{
		ImagePrice1K: &price1K,
		// ImagePrice2K 和 ImagePrice4K 为 nil，不更新
REDACTED

	group, err := svc.UpdateGroup(context.Background(), 1, input)
REDACTED
	require.NotNil(t, group)

	// 验证：1K 被更新，2K 保持原值，4K 仍为 nil
	require.NotNil(t, repo.updated)
	require.NotNil(t, repo.updated.ImagePrice1K)
	require.InDelta(t, 0.10, *repo.updated.ImagePrice1K, 0.0001)
	require.NotNil(t, repo.updated.ImagePrice2K)
	require.InDelta(t, 0.15, *repo.updated.ImagePrice2K, 0.0001) // 原值保持
	require.Nil(t, repo.updated.ImagePrice4K)
REDACTED

func TestAdminService_ListGroups_WithSearch(t *testing.T) {
	// 测试：
	// 1. search 参数正常传递到 repository 层
	// 2. search 为空字符串时的行为
	// 3. search 与其他过滤条件组合使用

	t.Run("search 参数正常传递到 repository 层", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{
			listWithFiltersGroups: []Group{{ID: 1, Name: "alpha"REDACTEDREDACTED,
			listWithFiltersResult: &pagination.PaginationResult{Total: 1REDACTED,
	REDACTED
		svc := &adminServiceImpl{groupRepo: repoREDACTED

		groups, total, err := svc.ListGroups(context.Background(), 1, 20, "", "", "alpha", nil)
	REDACTED
		require.Equal(t, int64(1), total)
		require.Equal(t, []Group{{ID: 1, Name: "alpha"REDACTEDREDACTED, groups)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 1, PageSize: 20REDACTED, repo.listWithFiltersParams)
		require.Equal(t, "alpha", repo.listWithFiltersSearch)
		require.Nil(t, repo.listWithFiltersIsExclusive)
REDACTED)

	t.Run("search 为空字符串时传递空字符串", func(t *testing.T) {
		repo := &groupRepoStubForAdmin{
			listWithFiltersGroups: []Group{REDACTED,
			listWithFiltersResult: &pagination.PaginationResult{Total: 0REDACTED,
	REDACTED
		svc := &adminServiceImpl{groupRepo: repoREDACTED

		groups, total, err := svc.ListGroups(context.Background(), 2, 10, "", "", "", nil)
	REDACTED
		require.Empty(t, groups)
		require.Equal(t, int64(0), total)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 2, PageSize: 10REDACTED, repo.listWithFiltersParams)
		require.Equal(t, "", repo.listWithFiltersSearch)
		require.Nil(t, repo.listWithFiltersIsExclusive)
REDACTED)

	t.Run("search 与其他过滤条件组合使用", func(t *testing.T) {
		isExclusive := true
		repo := &groupRepoStubForAdmin{
			listWithFiltersGroups: []Group{{ID: 2, Name: "beta"REDACTEDREDACTED,
			listWithFiltersResult: &pagination.PaginationResult{Total: 42REDACTED,
	REDACTED
		svc := &adminServiceImpl{groupRepo: repoREDACTED

		groups, total, err := svc.ListGroups(context.Background(), 3, 50, PlatformAntigravity, StatusActive, "beta", &isExclusive)
	REDACTED
		require.Equal(t, int64(42), total)
		require.Equal(t, []Group{{ID: 2, Name: "beta"REDACTEDREDACTED, groups)

		require.Equal(t, 1, repo.listWithFiltersCalls)
		require.Equal(t, pagination.PaginationParams{Page: 3, PageSize: 50REDACTED, repo.listWithFiltersParams)
		require.Equal(t, PlatformAntigravity, repo.listWithFiltersPlatform)
		require.Equal(t, StatusActive, repo.listWithFiltersStatus)
		require.Equal(t, "beta", repo.listWithFiltersSearch)
		require.NotNil(t, repo.listWithFiltersIsExclusive)
		require.True(t, *repo.listWithFiltersIsExclusive)
REDACTED)
REDACTED

func TestAdminService_ValidateFallbackGroup_DetectsCycle(t *testing.T) {
	groupID := int64(1)
	fallbackID := int64(2)
	repo := &groupRepoStubForFallbackCycle{
		groups: map[int64]*Group{
			groupID: {
				ID:              groupID,
				FallbackGroupID: &fallbackID,
		REDACTED,
			fallbackID: {
				ID:              fallbackID,
				FallbackGroupID: &groupID,
		REDACTED,
	REDACTED,
REDACTED
	svc := &adminServiceImpl{groupRepo: repoREDACTED

	err := svc.validateFallbackGroup(context.Background(), groupID, fallbackID)
REDACTED
	require.Contains(t, err.Error(), "fallback group cycle")
REDACTED

type groupRepoStubForFallbackCycle struct {
	groups map[int64]*Group
REDACTED

func (s *groupRepoStubForFallbackCycle) Create(_ context.Context, _ *Group) error {
	panic("unexpected Create call")
REDACTED

func (s *groupRepoStubForFallbackCycle) Update(_ context.Context, _ *Group) error {
	panic("unexpected Update call")
REDACTED

func (s *groupRepoStubForFallbackCycle) GetByID(ctx context.Context, id int64) (*Group, error) {
	return s.GetByIDLite(ctx, id)
REDACTED

func (s *groupRepoStubForFallbackCycle) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	if g, ok := s.groups[id]; ok {
		return g, nil
REDACTED
	return nil, ErrGroupNotFound
REDACTED

func (s *groupRepoStubForFallbackCycle) Delete(_ context.Context, _ int64) error {
	panic("unexpected Delete call")
REDACTED

func (s *groupRepoStubForFallbackCycle) DeleteCascade(_ context.Context, _ int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
REDACTED

func (s *groupRepoStubForFallbackCycle) List(_ context.Context, _ pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
REDACTED

func (s *groupRepoStubForFallbackCycle) ListWithFilters(_ context.Context, _ pagination.PaginationParams, _, _, _ string, _ *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
REDACTED

func (s *groupRepoStubForFallbackCycle) ListActive(_ context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
REDACTED

func (s *groupRepoStubForFallbackCycle) ListActiveByPlatform(_ context.Context, _ string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
REDACTED

func (s *groupRepoStubForFallbackCycle) ExistsByName(_ context.Context, _ string) (bool, error) {
	panic("unexpected ExistsByName call")
REDACTED

func (s *groupRepoStubForFallbackCycle) GetAccountCount(_ context.Context, _ int64) (int64, error) {
	panic("unexpected GetAccountCount call")
REDACTED

func (s *groupRepoStubForFallbackCycle) DeleteAccountGroupsByGroupID(_ context.Context, _ int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
REDACTED
