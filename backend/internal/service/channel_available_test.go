//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// stubGroupRepoForAvailable 是 ListAvailable 测试用的 GroupRepository stub，
// 仅实现 ListActive；其他方法对本测试无关，返回零值即可。
// listActiveErr 非 nil 时，ListActive 返回该错误用于错误传播测试。
// listActiveCalls 记录调用次数，用于断言「失败短路时不再访问 groupRepo」等行为。
type stubGroupRepoForAvailable struct {
	activeGroups    []Group
	listActiveErr   error
	listActiveCalls int
}

func (s *stubGroupRepoForAvailable) ListActive(ctx context.Context) ([]Group, error) {
	s.listActiveCalls++
	if s.listActiveErr != nil {
		return nil, s.listActiveErr
	}
	return s.activeGroups, nil
}

func (s *stubGroupRepoForAvailable) Create(ctx context.Context, group *Group) error { return nil }
func (s *stubGroupRepoForAvailable) GetByID(ctx context.Context, id int64) (*Group, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) Update(ctx context.Context, group *Group) error { return nil }
func (s *stubGroupRepoForAvailable) Delete(ctx context.Context, id int64) error     { return nil }
func (s *stubGroupRepoForAvailable) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) List(ctx context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubGroupRepoForAvailable) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (s *stubGroupRepoForAvailable) ListActiveByPlatform(ctx context.Context, platform string) ([]Group, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, nil
}
func (s *stubGroupRepoForAvailable) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	return 0, 0, nil
}
func (s *stubGroupRepoForAvailable) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
}
func (s *stubGroupRepoForAvailable) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, nil
}
func (s *stubGroupRepoForAvailable) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return nil
}
func (s *stubGroupRepoForAvailable) UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	return nil
}

// newAvailableChannelService 构造一个 ChannelService，channelRepo.ListAll 返回给定 channels，
// groupRepo 由参数决定。传入空 stub 表示「活跃分组列表为空」。
func newAvailableChannelService(channels []Channel, groupRepo GroupRepository) *ChannelService {
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return channels, nil },
	}
	return NewChannelService(repo, groupRepo, nil, nil)
}

func TestListAvailable_EmptyActiveGroups_NoGroupsAttached(t *testing.T) {
	// 活跃分组列表为空时，渠道的 Groups 应为空切片，不报错。
	channels := []Channel{{
		ID:       1,
		Name:     "chA",
		Status:   StatusActive,
		GroupIDs: []int64{10, 20},
	}}
	svc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{})
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Empty(t, out[0].Groups)
}

func TestListAvailable_InactiveGroupIDSilentlyDropped(t *testing.T) {
	// 渠道 GroupIDs 中引用的 group 未出现在 ListActive 结果中（已停用或删除），应被静默丢弃。
	channels := []Channel{{
		ID:       1,
		Name:     "chA",
		Status:   StatusActive,
		GroupIDs: []int64{1, 99},
	}}
	groupRepo := &stubGroupRepoForAvailable{
		activeGroups: []Group{{ID: 1, Name: "g1", Platform: "anthropic"}},
	}
	svc := newAvailableChannelService(channels, groupRepo)
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Len(t, out[0].Groups, 1)
	require.Equal(t, int64(1), out[0].Groups[0].ID)
}

func TestListAvailable_SortedByName(t *testing.T) {
	channels := []Channel{
		{ID: 1, Name: "beta"},
		{ID: 2, Name: "Alpha"},
		{ID: 3, Name: "charlie"},
	}
	svc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{})
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, "Alpha", out[0].Name)
	require.Equal(t, "beta", out[1].Name)
	require.Equal(t, "charlie", out[2].Name)
}

func TestListAvailable_ListAllErrorPropagates(t *testing.T) {
	// ListAll 返回错误时 ListAvailable 应直接返回包装后的错误，且不再访问 groupRepo（短路）。
	sentinel := errors.New("list-all-boom")
	repo := &mockChannelRepository{
		listAllFn: func(ctx context.Context) ([]Channel, error) { return nil, sentinel },
	}
	groupRepo := &stubGroupRepoForAvailable{}
	svc := NewChannelService(repo, groupRepo, nil, nil)
	out, err := svc.ListAvailable(context.Background())
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "list channels", "wrap 前缀缺失，可能 %w 被改为 %v")
	require.Equal(t, 0, groupRepo.listActiveCalls, "ListAll 失败后不应再调用 groupRepo.ListActive")
}

func TestListAvailable_ListActiveErrorPropagates(t *testing.T) {
	// groupRepo.ListActive 返回错误时 ListAvailable 应直接返回包装后的错误。
	sentinel := errors.New("list-active-boom")
	svc := newAvailableChannelService(
		[]Channel{{ID: 1, Name: "chA"}},
		&stubGroupRepoForAvailable{listActiveErr: sentinel},
	)
	out, err := svc.ListAvailable(context.Background())
	require.Nil(t, out)
	require.ErrorIs(t, err, sentinel)
	require.Contains(t, err.Error(), "list active groups", "wrap 前缀缺失，可能 %w 被改为 %v")
}

func TestListAvailable_DefaultsEmptyBillingModelSource(t *testing.T) {
	// 渠道 BillingModelSource 为空时应回填为 BillingModelSourceChannelMapped，
	// 显式值应原样保留（由 service 层统一处理，避免各 handler 重复默认逻辑）。
	channels := []Channel{
		{ID: 1, Name: "empty", BillingModelSource: ""},
		{ID: 2, Name: "explicit", BillingModelSource: BillingModelSourceUpstream},
	}
	svc := newAvailableChannelService(channels, &stubGroupRepoForAvailable{})
	out, err := svc.ListAvailable(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)

	// 按 Name 查找，避免依赖排序副作用。
	byName := make(map[string]string, len(out))
	for _, ch := range out {
		byName[ch.Name] = ch.BillingModelSource
	}
	require.Equal(t, BillingModelSourceChannelMapped, byName["empty"])
	require.Equal(t, BillingModelSourceUpstream, byName["explicit"])
}
