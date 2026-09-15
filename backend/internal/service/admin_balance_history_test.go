package service

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
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

	got := mergeBalanceHistoryCodes(pagination.PaginationParams{
		Page:     1,
		PageSize: 2,
	}, redeemCodes, affiliateCodes)

	require.Len(t, got, 2)
	require.Equal(t, RedeemTypeAffiliateBalance, got[0].Type)
	require.Equal(t, RedeemTypeBalance, got[1].Type)
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
		pagination.PaginationParams{Page: 2, PageSize: 2},
		[]RedeemCode{
			{ID: 1, Type: RedeemTypeBalance, UsedBy: &usedBy, UsedAt: at(4), CreatedAt: *at(4)},
			{ID: 2, Type: RedeemTypeInvitation, UsedBy: &usedBy, UsedAt: at(1), CreatedAt: *at(1)},
		},
		[]RedeemCode{
			{ID: -3, Type: RedeemTypeAffiliateBalance, UsedBy: &usedBy, UsedAt: at(3), CreatedAt: *at(3)},
		},
		[]RedeemCode{
			{ID: 4, Type: RedeemTypePromoBalance, UsedBy: &usedBy, UsedAt: at(2), CreatedAt: *at(2)},
		},
		[]RedeemCode{
			{ID: 5, Type: RedeemTypeLotteryBalance, UsedBy: &usedBy, UsedAt: at(0), CreatedAt: *at(0)},
		},
	)

	require.Len(t, got, 2)
	require.Equal(t, RedeemTypePromoBalance, got[0].Type)
	require.Equal(t, RedeemTypeInvitation, got[1].Type)
}

func TestListPromoBalanceHistoryMapsSignupPromoUsage(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	usedAt := time.Date(2026, 9, 5, 10, 30, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)FROM promo_code_usages pcu.*pcu\.user_id = \$1.*pcu\.bonus_amount > 0`).
		WithArgs(int64(42), 0, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "code", "bonus_amount", "notes", "used_at"}).
			AddRow(int64(7), "WELCOME", 3.5, "signup campaign", usedAt))
	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*FROM promo_code_usages.*user_id = \$1.*bonus_amount > 0`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	svc := &adminServiceImpl{entClient: client}
	codes, total, err := svc.listPromoBalanceHistory(context.Background(), 42, pagination.PaginationParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, codes, 1)
	require.Equal(t, RedeemTypePromoBalance, codes[0].Type)
	require.Equal(t, "WELCOME", codes[0].Code)
	require.Equal(t, 3.5, codes[0].Value)
	require.Equal(t, "signup campaign", codes[0].Notes)
	require.Equal(t, &usedAt, codes[0].UsedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListLotteryBalanceHistoryMapsPositivePrize(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	drawnAt := time.Date(2026, 9, 5, 11, 0, 0, 0, time.UTC)
	createdAt := drawnAt.Add(-time.Minute)
	mock.ExpectQuery(`(?s)FROM daily_lottery_entries.*user_id = \$1.*drawn_at IS NOT NULL.*reward_amount > 0`).
		WithArgs(int64(42), 20, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "checkin_date", "reward_amount", "prize_name", "drawn_at", "created_at"}).
			AddRow(int64(9), "2026-09-05", 1.25, "一等奖", drawnAt, createdAt))
	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*FROM daily_lottery_entries.*user_id = \$1.*drawn_at IS NOT NULL.*reward_amount > 0`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))

	svc := &adminServiceImpl{entClient: client}
	codes, total, err := svc.listLotteryBalanceHistory(context.Background(), 42, pagination.PaginationParams{Page: 2, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, codes, 1)
	require.Equal(t, RedeemTypeLotteryBalance, codes[0].Type)
	require.Equal(t, "LOTTERY-2026-09-05", codes[0].Code)
	require.Equal(t, 1.25, codes[0].Value)
	require.Equal(t, "一等奖", codes[0].Notes)
	require.Equal(t, &drawnAt, codes[0].UsedAt)
	require.Equal(t, createdAt, codes[0].CreatedAt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSumUserBalanceCreditsIncludesEveryPersistedPositiveSource(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })

	mock.ExpectQuery(`(?s)SELECT \(.*FROM redeem_codes.*value > 0.*type IN \('balance', 'admin_balance'\).*FROM user_affiliate_ledger.*action = 'transfer'.*amount > 0.*FROM promo_code_usages.*bonus_amount > 0.*FROM daily_lottery_entries.*drawn_at IS NOT NULL.*reward_amount > 0`).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(15.75))

	svc := &adminServiceImpl{entClient: client}
	total, err := svc.sumUserBalanceCredits(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, 15.75, total)
	require.NoError(t, mock.ExpectationsWereMet())
}
