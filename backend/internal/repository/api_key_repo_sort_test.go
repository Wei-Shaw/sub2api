package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepositoryListByUserIDSortByGroup(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "group-sort@test.com")
	otherUser := mustCreateAPIKeyRepoUser(t, ctx, client, "other-group-sort@test.com")
	createGroup := func(name string) *dbent.Group {
		g, err := client.Group.Create().SetName(name).Save(ctx)
		require.NoError(t, err)
		return g
	}
	// Create groups in reverse name order so sorting by group ID cannot pass.
	zulu := createGroup("Zulu")
	alpha := createGroup("Alpha")
	createKey := func(userID int64, name string, groupID *int64, status string) int64 {
		key := &service.APIKey{
			UserID: userID, Key: "sk-" + name, Name: name, GroupID: groupID, Status: status,
		}
		require.NoError(t, repo.Create(ctx, key))
		return key.ID
	}
	zuluFirst := createKey(user.ID, "match-zulu-first", &zulu.ID, service.StatusActive)
	ungroupedFirst := createKey(user.ID, "match-ungrouped-first", nil, service.StatusActive)
	alphaFirst := createKey(user.ID, "match-alpha-first", &alpha.ID, service.StatusActive)
	zuluSecond := createKey(user.ID, "match-zulu-second", &zulu.ID, service.StatusActive)
	alphaSecond := createKey(user.ID, "match-alpha-second", &alpha.ID, service.StatusActive)
	ungroupedSecond := createKey(user.ID, "match-ungrouped-second", nil, service.StatusActive)
	createKey(otherUser.ID, "match-other-user", &alpha.ID, service.StatusActive)
	createKey(user.ID, "excluded-search", &alpha.ID, service.StatusActive)
	createKey(user.ID, "match-inactive", &alpha.ID, service.StatusDisabled)
	deleted := createKey(user.ID, "match-deleted", &alpha.ID, service.StatusActive)
	require.NoError(t, repo.Delete(ctx, deleted))

	ungroupedID := int64(0)
	for _, tc := range []struct {
		name    string
		order   string
		groupID *int64
		want    []int64
	}{
		{"ascending", "asc", nil, []int64{alphaFirst, alphaSecond, zuluFirst, zuluSecond, ungroupedFirst, ungroupedSecond}},
		{"descending", "desc", nil, []int64{zuluSecond, zuluFirst, alphaSecond, alphaFirst, ungroupedSecond, ungroupedFirst}},
		{"group filter", "asc", &alpha.ID, []int64{alphaFirst, alphaSecond}},
		{"ungrouped filter", "desc", &ungroupedID, []int64{ungroupedSecond, ungroupedFirst}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []int64
			const pageSize = 3
			pages := (len(tc.want) + pageSize - 1) / pageSize
			for page := 1; page <= pages; page++ {
				keys, result, err := repo.ListByUserID(ctx, user.ID, pagination.PaginationParams{
					Page: page, PageSize: pageSize, SortBy: "group", SortOrder: tc.order,
				}, service.APIKeyListFilters{Search: "match", Status: service.StatusActive, GroupID: tc.groupID})
				require.NoError(t, err)
				require.EqualValues(t, len(tc.want), result.Total)
				require.Equal(t, pages, result.Pages)
				for _, key := range keys {
					got = append(got, key.ID)
					if key.GroupID != nil {
						require.NotNil(t, key.Group, "sorting must preserve group preloading")
					}
				}
			}
			require.Equal(t, tc.want, got)
		})
	}
}
