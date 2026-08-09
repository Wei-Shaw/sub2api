package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type publicGroupsRepoStub struct {
	GroupRepository
	pages      map[int][]Group
	results    map[int]*pagination.PaginationResult
	err        error
	errs       map[int]error
	calls      []pagination.PaginationParams
	platforms  []string
	statuses   []string
	searches   []string
	exclusives []*bool
}

func (s *publicGroupsRepoStub) ListWithFilters(
	_ context.Context,
	params pagination.PaginationParams,
	platform, status, search string,
	isExclusive *bool,
) ([]Group, *pagination.PaginationResult, error) {
	s.calls = append(s.calls, params)
	s.platforms = append(s.platforms, platform)
	s.statuses = append(s.statuses, status)
	s.searches = append(s.searches, search)
	s.exclusives = append(s.exclusives, isExclusive)
	if s.err != nil {
		return nil, nil, s.err
	}
	if err := s.errs[params.Page]; err != nil {
		return nil, nil, err
	}
	return s.pages[params.Page], s.results[params.Page], nil
}

func TestGroupServiceListPublic_LoadsEveryPageAndRequestsNonExclusive(t *testing.T) {
	repo := &publicGroupsRepoStub{
		pages: map[int][]Group{
			1: {{ID: 1, Name: "public-active"}},
			2: {{ID: 2, Name: "public-disabled", Status: StatusDisabled}},
		},
		results: map[int]*pagination.PaginationResult{
			1: {Page: 1, PageSize: 1000, Pages: 2, Total: 2},
			2: {Page: 2, PageSize: 1000, Pages: 2, Total: 2},
		},
	}

	groups, err := NewGroupService(repo, nil).ListPublic(context.Background())
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, []int64{groups[0].ID, groups[1].ID})
	require.Len(t, repo.calls, 2)
	for i := range repo.calls {
		require.Equal(t, i+1, repo.calls[i].Page)
		require.Equal(t, 1000, repo.calls[i].PageSize)
		require.Equal(t, "sort_order", repo.calls[i].SortBy)
		require.Equal(t, pagination.SortOrderAsc, repo.calls[i].SortOrder)
		require.Empty(t, repo.platforms[i])
		require.Empty(t, repo.statuses[i])
		require.Empty(t, repo.searches[i])
		require.NotNil(t, repo.exclusives[i])
		require.False(t, *repo.exclusives[i], "status page must request only public groups")
	}
}

func TestGroupServiceListPublic_NilPaginationResultStopsAfterCurrentPage(t *testing.T) {
	repo := &publicGroupsRepoStub{
		pages: map[int][]Group{1: {{ID: 1, Name: "public"}}},
	}

	groups, err := NewGroupService(repo, nil).ListPublic(context.Background())
	require.NoError(t, err)
	require.Equal(t, []Group{{ID: 1, Name: "public"}}, groups)
	require.Len(t, repo.calls, 1)
}

func TestGroupServiceListPublic_ContinuesAfterEmptyIntermediatePage(t *testing.T) {
	repo := &publicGroupsRepoStub{
		pages: map[int][]Group{
			1: {{ID: 1, Name: "first"}},
			2: {},
			3: {{ID: 3, Name: "last"}},
		},
		results: map[int]*pagination.PaginationResult{
			1: {Page: 1, PageSize: 1000, Pages: 3, Total: 2},
			2: {Page: 2, PageSize: 1000, Pages: 3, Total: 2},
			3: {Page: 3, PageSize: 1000, Pages: 3, Total: 2},
		},
	}

	groups, err := NewGroupService(repo, nil).ListPublic(context.Background())
	require.NoError(t, err)
	require.Equal(t, []int64{1, 3}, []int64{groups[0].ID, groups[1].ID})
	require.Len(t, repo.calls, 3)
}

func TestGroupServiceListPublic_PropagatesRepositoryError(t *testing.T) {
	repo := &publicGroupsRepoStub{err: errors.New("database unavailable")}
	groups, err := NewGroupService(repo, nil).ListPublic(context.Background())
	require.Nil(t, groups)
	require.ErrorContains(t, err, "list public groups")
}

func TestGroupServiceListPublic_PropagatesSecondPageErrorWithoutPartialResult(t *testing.T) {
	repo := &publicGroupsRepoStub{
		pages: map[int][]Group{1: {{ID: 1, Name: "first"}}},
		results: map[int]*pagination.PaginationResult{
			1: {Page: 1, PageSize: 1000, Pages: 2, Total: 2},
		},
		errs: map[int]error{2: errors.New("second page unavailable")},
	}

	groups, err := NewGroupService(repo, nil).ListPublic(context.Background())
	require.Nil(t, groups)
	require.ErrorContains(t, err, "list public groups")
	require.ErrorContains(t, err, "second page unavailable")
	require.Len(t, repo.calls, 2)
}
