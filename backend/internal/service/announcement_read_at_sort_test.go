package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// readByReadAtRepoStub captures the call to ListUsersOrderedByReadAt and returns
// canned rows. The other methods are required by the interface but not exercised
// by the read-at sort path under test.
type readByReadAtRepoStub struct {
	called   bool
	gotAnnc  int64
	gotParam pagination.PaginationParams
	gotSrch  string
	rows     []AnnouncementReadUserRow
	page     *pagination.PaginationResult
}

func (r *readByReadAtRepoStub) MarkRead(context.Context, int64, int64, time.Time) error {
	return nil
}
func (r *readByReadAtRepoStub) GetReadMapByUser(context.Context, int64, []int64) (map[int64]time.Time, error) {
	return map[int64]time.Time{}, nil
}
func (r *readByReadAtRepoStub) GetReadMapByUsers(context.Context, int64, []int64) (map[int64]time.Time, error) {
	return map[int64]time.Time{}, nil
}
func (r *readByReadAtRepoStub) CountByAnnouncementID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (r *readByReadAtRepoStub) ListUsersOrderedByReadAt(
	_ context.Context,
	announcementID int64,
	params pagination.PaginationParams,
	search string,
) ([]AnnouncementReadUserRow, *pagination.PaginationResult, error) {
	r.called = true
	r.gotAnnc = announcementID
	r.gotParam = params
	r.gotSrch = search
	return r.rows, r.page, nil
}

// userSubRepoNoSubs is a UserSubscriptionRepository stub that always returns no
// active subscriptions (so eligibility computation in service degrades to the
// targeting's "no AnyOf -> match all" branch).
type userSubRepoNoSubs struct{ UserSubscriptionRepository }

func (userSubRepoNoSubs) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return nil, nil
}

func TestListUserReadStatus_RoutesToReadAtPath(t *testing.T) {
	t.Parallel()

	annRepo := &announcementRepoStub{item: &Announcement{ID: 1, Title: "x", Content: "x"}}
	readRepo := &readByReadAtRepoStub{
		rows: []AnnouncementReadUserRow{
			{UserID: 7, Email: "early@example.com", Username: "early", Balance: 0, ReadAt: timePtr(time.Unix(1000, 0))},
			{UserID: 8, Email: "late@example.com", Username: "late", Balance: 0, ReadAt: timePtr(time.Unix(2000, 0))},
			{UserID: 9, Email: "unread@example.com", Username: "unread", Balance: 0, ReadAt: nil},
		},
		page: &pagination.PaginationResult{Page: 1, PageSize: 20, Total: 3, Pages: 1},
	}
	svc := NewAnnouncementService(annRepo, readRepo, nil, userSubRepoNoSubs{})

	params := pagination.PaginationParams{
		Page:      1,
		PageSize:  20,
		SortBy:    "read_at",
		SortOrder: "asc",
	}
	out, page, err := svc.ListUserReadStatus(context.Background(), 1, params, "")
	require.NoError(t, err)

	require.True(t, readRepo.called, "expected service to dispatch to ListUsersOrderedByReadAt when sort_by=read_at")
	require.Equal(t, int64(1), readRepo.gotAnnc)
	require.Equal(t, params, readRepo.gotParam)

	require.Len(t, out, 3)
	require.Equal(t, int64(7), out[0].UserID)
	require.Equal(t, int64(8), out[1].UserID)
	require.Equal(t, int64(9), out[2].UserID)
	require.NotNil(t, out[0].ReadAt)
	require.Nil(t, out[2].ReadAt)

	require.NotNil(t, page)
	require.Equal(t, int64(3), page.Total)
}

func TestListUserReadStatus_DefaultPathWhenSortByOther(t *testing.T) {
	t.Parallel()

	annRepo := &announcementRepoStub{item: &Announcement{ID: 1, Title: "x", Content: "x"}}
	readRepo := &readByReadAtRepoStub{}
	// userRepo is nil → the default path would attempt ListWithFilters and crash.
	// We only care that the read-at path is NOT taken; assert via readRepo.called.
	defer func() {
		_ = recover() // swallow the nil-pointer panic from the default path
		require.False(t, readRepo.called, "expected default path when sort_by != read_at")
	}()
	svc := NewAnnouncementService(annRepo, readRepo, nil, nil)

	_, _, _ = svc.ListUserReadStatus(context.Background(), 1, pagination.PaginationParams{
		Page: 1, PageSize: 20, SortBy: "email", SortOrder: "asc",
	}, "")
}

func TestListUserReadStatus_TrimsAndLowercasesSortBy(t *testing.T) {
	t.Parallel()

	annRepo := &announcementRepoStub{item: &Announcement{ID: 1, Title: "x", Content: "x"}}
	readRepo := &readByReadAtRepoStub{
		page: &pagination.PaginationResult{Page: 1, PageSize: 20, Total: 0, Pages: 0},
	}
	svc := NewAnnouncementService(annRepo, readRepo, nil, userSubRepoNoSubs{})

	// "  READ_AT  " should still route to the read-at path.
	_, _, err := svc.ListUserReadStatus(context.Background(), 1, pagination.PaginationParams{
		Page: 1, PageSize: 20, SortBy: "  READ_AT  ", SortOrder: "desc",
	}, "")
	require.NoError(t, err)
	require.True(t, readRepo.called)
}

func timePtr(t time.Time) *time.Time { return &t }
