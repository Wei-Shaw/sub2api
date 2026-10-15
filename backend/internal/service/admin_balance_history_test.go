package service

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestMergeBalanceHistoryCodesIncludesAffiliateTransfersByDefault(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	older := now.Add(-2 * time.Hour)
	newer := now.Add(time.Hour)

	usedBy := int64(10)
	redeemCodes := []RedeemCode{
		{
			ID:        1,
			Type:      RedeemTypeBalance,
			Value:     8,
			Status:    StatusUsed,
			UsedBy:    &usedBy,
			UsedAt:    &now,
			CreatedAt: now,
		},
		{
			ID:        2,
			Type:      RedeemTypeConcurrency,
			Value:     1,
			Status:    StatusUsed,
			UsedBy:    &usedBy,
			UsedAt:    &older,
			CreatedAt: older,
		},
	}
	affiliateCodes := []RedeemCode{
		{
			ID:        -20,
			Type:      RedeemTypeAffiliateBalance,
			Value:     3.5,
			Status:    StatusUsed,
			UsedBy:    &usedBy,
			UsedAt:    &newer,
			CreatedAt: newer,
		},
	}

	checkInCodes := []RedeemCode{
		{
			ID:        30,
			Type:      RedeemTypeCheckInBalance,
			Value:     6,
			Status:    StatusUsed,
			UsedBy:    &usedBy,
			UsedAt:    &newer,
			CreatedAt: newer,
		},
	}

	got := mergeBalanceHistoryCodes(redeemCodes, affiliateCodes, checkInCodes, pagination.PaginationParams{
		Page:     1,
		PageSize: 2,
	})

	require.Len(t, got, 2)
	require.Equal(t, RedeemTypeAffiliateBalance, got[0].Type)
	require.Equal(t, RedeemTypeCheckInBalance, got[1].Type)
}

func TestMergeBalanceHistoryCodesPaginatesAfterCombiningSources(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	usedBy := int64(10)
	at := func(hours int) *time.Time {
		v := base.Add(time.Duration(hours) * time.Hour)
		return &v
	}

	got := mergeBalanceHistoryCodes(
		[]RedeemCode{
			{ID: 1, Type: RedeemTypeBalance, UsedBy: &usedBy, UsedAt: at(4), CreatedAt: *at(4)},
			{ID: 2, Type: RedeemTypeConcurrency, UsedBy: &usedBy, UsedAt: at(2), CreatedAt: *at(2)},
		},
		[]RedeemCode{
			{ID: -3, Type: RedeemTypeAffiliateBalance, UsedBy: &usedBy, UsedAt: at(3), CreatedAt: *at(3)},
			{ID: -4, Type: RedeemTypeAffiliateBalance, UsedBy: &usedBy, UsedAt: at(1), CreatedAt: *at(1)},
		},
		[]RedeemCode{
			{ID: 5, Type: RedeemTypeCheckInBalance, UsedBy: &usedBy, UsedAt: at(5), CreatedAt: *at(5)},
		},
		pagination.PaginationParams{Page: 2, PageSize: 2},
	)

	require.Len(t, got, 2)
	require.Equal(t, RedeemTypeAffiliateBalance, got[0].Type)
	require.Equal(t, RedeemTypeConcurrency, got[1].Type)
}

func TestListCheckInBalanceHistoryMapsRecords(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })

	createdAt := time.Date(2026, 8, 17, 10, 30, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id,
       reward::double precision,
       created_at
FROM user_checkins
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
OFFSET $2
LIMIT $3`)).
		WithArgs(int64(10), 0, 15).
		WillReturnRows(sqlmock.NewRows([]string{"id", "reward", "created_at"}).AddRow(int64(23), 6.0, createdAt))
	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT COUNT(*)
FROM user_checkins
WHERE user_id = $1`)).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	svc := &adminServiceImpl{entClient: client}
	codes, total, err := svc.listCheckInBalanceHistory(context.Background(), 10, pagination.PaginationParams{Page: 1, PageSize: 15})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, codes, 1)
	require.Equal(t, int64(23), codes[0].ID)
	require.Equal(t, "CHECKIN-23", codes[0].Code)
	require.Equal(t, RedeemTypeCheckInBalance, codes[0].Type)
	require.Equal(t, 6.0, codes[0].Value)
	require.Equal(t, StatusUsed, codes[0].Status)
	require.Equal(t, int64(10), *codes[0].UsedBy)
	require.Equal(t, createdAt, *codes[0].UsedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}
