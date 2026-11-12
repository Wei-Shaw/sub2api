//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type groupRepoStubForGroupServiceDelete struct {
	getByIDGroup       *Group
	getByIDErr         error
	deleteCalls        []int64
	deleteCascadeCalls []int64
	deleteErr          error
	deleteCascadeErr   error
}

func (s *groupRepoStubForGroupServiceDelete) Create(context.Context, *Group) error {
	panic("unexpected Create call")
}

func (s *groupRepoStubForGroupServiceDelete) GetByID(context.Context, int64) (*Group, error) {
	if s.getByIDErr != nil {
		return nil, s.getByIDErr
	}
	return s.getByIDGroup, nil
}

func (s *groupRepoStubForGroupServiceDelete) GetByIDLite(context.Context, int64) (*Group, error) {
	panic("unexpected GetByIDLite call")
}

func (s *groupRepoStubForGroupServiceDelete) Update(context.Context, *Group) error {
	panic("unexpected Update call")
}

func (s *groupRepoStubForGroupServiceDelete) Delete(_ context.Context, id int64) error {
	s.deleteCalls = append(s.deleteCalls, id)
	return s.deleteErr
}

func (s *groupRepoStubForGroupServiceDelete) DeleteCascade(_ context.Context, id int64) ([]int64, error) {
	s.deleteCascadeCalls = append(s.deleteCascadeCalls, id)
	return nil, s.deleteCascadeErr
}

func (s *groupRepoStubForGroupServiceDelete) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *groupRepoStubForGroupServiceDelete) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *groupRepoStubForGroupServiceDelete) ListActive(context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
}

func (s *groupRepoStubForGroupServiceDelete) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}

func (s *groupRepoStubForGroupServiceDelete) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected ExistsByName call")
}

func (s *groupRepoStubForGroupServiceDelete) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}

func (s *groupRepoStubForGroupServiceDelete) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}

func (s *groupRepoStubForGroupServiceDelete) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}

func (s *groupRepoStubForGroupServiceDelete) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected BindAccountsToGroup call")
}

func (s *groupRepoStubForGroupServiceDelete) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	panic("unexpected UpdateSortOrders call")
}

func TestGroupService_Delete_UsesDeleteCascade(t *testing.T) {
	repo := &groupRepoStubForGroupServiceDelete{
		getByIDGroup: &Group{ID: 6, Name: "All"},
	}
	invalidator := &authCacheInvalidatorStub{}
	svc := NewGroupService(repo, invalidator)

	err := svc.Delete(context.Background(), 6)
	require.NoError(t, err)
	require.Equal(t, []int64{6}, repo.deleteCascadeCalls)
	require.Empty(t, repo.deleteCalls)
	require.Equal(t, []int64{6}, invalidator.groupIDs)
}
