package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestBotSalesHistoryIncludesCompletedAndLegacyFulfillments(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	svc := &adminServiceImpl{entClient: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
	completed := time.Date(2026, 10, 28, 13, 10, 2, 0, time.UTC)
	legacy := completed.Add(-24 * time.Hour)
	mock.ExpectQuery(`(?s)SELECT f\.id,.*COALESCE\(o\.completed_at, f\.updated_at\).*FROM bot_sales_fulfillments f.*LEFT JOIN payment_orders o.*WHERE f.user_id = \$1.*f.status = 'succeeded'.*ORDER BY COALESCE\(o\.completed_at, f\.updated_at\) DESC, f\.id DESC.*OFFSET \$2.*LIMIT \$3`).
		WithArgs(int64(685), 0, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "external_order_code", "external_order_item_id", "balance_amount", "completed_at", "created_at", "payment_order_id"}).
			AddRow(int64(326), "BS_1793192965095", "2898", 110.0, completed, completed.Add(-time.Minute), int64(1075)).
			AddRow(int64(1), "BS_LEGACY", "old-item", 10.0, legacy, legacy.Add(-time.Minute), nil))
	mock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*FROM bot_sales_fulfillments.*WHERE user_id = \$1.*status = 'succeeded'`).
		WithArgs(int64(685)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))

	codes, total, err := svc.listBotSalesBalanceHistory(context.Background(), 685, pagination.PaginationParams{Page: 1, PageSize: 2})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, codes, 2)
	require.Equal(t, RedeemTypeBotSalesBalance, codes[0].Type)
	require.Equal(t, "BS_1793192965095", codes[0].Code)
	require.Equal(t, "2898", codes[0].ExternalOrderItemID)
	require.Equal(t, int64(1075), *codes[0].PaymentOrderID)
	require.Equal(t, 110.0, codes[0].Value)
	require.Equal(t, completed, *codes[0].UsedAt)
	require.Equal(t, int64(-1000000000326), codes[0].ID)
	require.Nil(t, codes[1].PaymentOrderID, "old fulfillment must remain visible without a synthetic payment order")
	require.NotEqual(t, codes[0].ID, codes[1].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBotSalesHistoryQueryErrorIsReturned(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	svc := &adminServiceImpl{entClient: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
	mock.ExpectQuery(`(?s)FROM bot_sales_fulfillments f`).WithArgs(int64(685), 0, 1).
		WillReturnError(errors.New("database unavailable"))
	_, _, err = svc.listBotSalesBalanceHistory(context.Background(), 685, pagination.PaginationParams{Page: 1, PageSize: 1})
	require.ErrorContains(t, err, "database unavailable")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMergeBalanceHistoryIncludesBotSalesAcrossPageBoundary(t *testing.T) {
	now := time.Date(2026, 10, 28, 13, 10, 2, 0, time.UTC)
	old := now.Add(-time.Minute)
	newer := now.Add(time.Minute)
	got := mergeBalanceHistoryCodes(
		[]RedeemCode{{ID: 1, Type: RedeemTypeBalance, UsedAt: &newer}},
		[]RedeemCode{{ID: -1, Type: RedeemTypeAffiliateBalance, UsedAt: &old}},
		[]RedeemCode{{ID: -1000000000326, Type: RedeemTypeBotSalesBalance, UsedAt: &now}},
		pagination.PaginationParams{Page: 2, PageSize: 1},
	)
	require.Len(t, got, 1)
	require.Equal(t, RedeemTypeBotSalesBalance, got[0].Type)
}

type balanceHistoryRedeemRepoStub struct {
	codes []RedeemCode
}

func (s *balanceHistoryRedeemRepoStub) Create(context.Context, *RedeemCode) error {
	return errors.New("unexpected Create")
}
func (s *balanceHistoryRedeemRepoStub) CreateBatch(context.Context, []RedeemCode) error {
	return errors.New("unexpected CreateBatch")
}
func (s *balanceHistoryRedeemRepoStub) GetByID(context.Context, int64) (*RedeemCode, error) {
	return nil, errors.New("unexpected GetByID")
}
func (s *balanceHistoryRedeemRepoStub) GetByCode(context.Context, string) (*RedeemCode, error) {
	return nil, errors.New("unexpected GetByCode")
}
func (s *balanceHistoryRedeemRepoStub) Update(context.Context, *RedeemCode) error {
	return errors.New("unexpected Update")
}
func (s *balanceHistoryRedeemRepoStub) BatchUpdate(context.Context, []int64, RedeemCodeBatchUpdateFields) (int64, error) {
	return 0, errors.New("unexpected BatchUpdate")
}
func (s *balanceHistoryRedeemRepoStub) Delete(context.Context, int64) error {
	return errors.New("unexpected Delete")
}
func (s *balanceHistoryRedeemRepoStub) Use(context.Context, int64, int64) error {
	return errors.New("unexpected Use")
}
func (s *balanceHistoryRedeemRepoStub) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("unexpected List")
}
func (s *balanceHistoryRedeemRepoStub) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("unexpected ListWithFilters")
}
func (s *balanceHistoryRedeemRepoStub) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	return nil, errors.New("unexpected ListByUser")
}
func (s *balanceHistoryRedeemRepoStub) ListByUserPaginated(_ context.Context, _ int64, params pagination.PaginationParams, _ string) ([]RedeemCode, *pagination.PaginationResult, error) {
	start := params.Offset()
	if start >= len(s.codes) {
		return []RedeemCode{}, &pagination.PaginationResult{Total: int64(len(s.codes))}, nil
	}
	end := start + params.Limit()
	if end > len(s.codes) {
		end = len(s.codes)
	}
	return s.codes[start:end], &pagination.PaginationResult{Total: int64(len(s.codes))}, nil
}
func (s *balanceHistoryRedeemRepoStub) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	return 50, nil
}

func TestGetUserBalanceHistoryPaginatesAcrossThreeSkewedSources(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	base := time.Date(2026, 10, 28, 13, 10, 0, 0, time.UTC)
	at := func(minutes int) *time.Time {
		value := base.Add(time.Duration(minutes) * time.Minute)
		return &value
	}
	usedBy := int64(685)
	redeemCodes := []RedeemCode{
		{ID: 1, Code: "R14", Type: RedeemTypeBalance, UsedBy: &usedBy, UsedAt: at(14)},
		{ID: 2, Code: "R11", Type: RedeemTypeBalance, UsedBy: &usedBy, UsedAt: at(11)},
		{ID: 3, Code: "R8", Type: RedeemTypeBalance, UsedBy: &usedBy, UsedAt: at(8)},
		{ID: 4, Code: "R5", Type: RedeemTypeBalance, UsedBy: &usedBy, UsedAt: at(5)},
		{ID: 5, Code: "R2", Type: RedeemTypeBalance, UsedBy: &usedBy, UsedAt: at(2)},
		{ID: 6, Code: "R-1", Type: RedeemTypeBalance, UsedBy: &usedBy, UsedAt: at(-1)},
	}
	service := &adminServiceImpl{
		entClient:      dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db))),
		redeemCodeRepo: &balanceHistoryRedeemRepoStub{codes: redeemCodes},
	}
	addSourceExpectations := func() {
		mock.ExpectQuery(`(?s)SELECT id,.*FROM user_affiliate_ledger.*ORDER BY created_at DESC, id DESC.*OFFSET \$2.*LIMIT \$3`).
			WithArgs(int64(685), 0, 1000).
			WillReturnRows(sqlmock.NewRows([]string{"id", "amount", "created_at"}).
				AddRow(int64(20), 1.0, *at(13)).
				AddRow(int64(21), 1.0, *at(10)).
				AddRow(int64(22), 1.0, *at(7)).
				AddRow(int64(23), 1.0, *at(4)))
		mock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*FROM user_affiliate_ledger.*action = 'transfer'`).
			WithArgs(int64(685)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(4)))
		mock.ExpectQuery(`(?s)SELECT f\.id,.*COALESCE\(o\.completed_at, f\.updated_at\).*FROM bot_sales_fulfillments f.*LEFT JOIN payment_orders o.*WHERE f.user_id = \$1.*f.status = 'succeeded'.*ORDER BY COALESCE\(o\.completed_at, f\.updated_at\) DESC, f\.id DESC.*OFFSET \$2.*LIMIT \$3`).
			WithArgs(int64(685), 0, 1000).
			WillReturnRows(sqlmock.NewRows([]string{"id", "external_order_code", "external_order_item_id", "balance_amount", "completed_at", "created_at", "payment_order_id"}).
				AddRow(int64(30), "B12", "30", 1.0, *at(12), *at(12), nil).
				AddRow(int64(31), "B9", "31", 1.0, *at(9), *at(9), nil).
				AddRow(int64(32), "B6", "32", 1.0, *at(6), *at(6), nil).
				AddRow(int64(33), "B3", "33", 1.0, *at(3), *at(3), nil).
				AddRow(int64(34), "B0", "34", 1.0, *at(0), *at(0), nil))
		mock.ExpectQuery(`(?s)SELECT COUNT\(\*\).*FROM bot_sales_fulfillments.*WHERE user_id = \$1.*status = 'succeeded'`).
			WithArgs(int64(685)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(5)))
		mock.ExpectQuery(`(?s)SUM\(balance_amount\).*FROM bot_sales_fulfillments.*WHERE user_id = \$1.*status = 'succeeded'.*balance_amount > 0`).
			WithArgs(int64(685)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(120.0))
	}

	for _, tc := range []struct {
		page int
		want []string
	}{
		{page: 2, want: []string{"B12", "R11"}},
		{page: 3, want: []string{"AFF-21", "B9"}},
		{page: 8, want: []string{"R-1"}},
	} {
		addSourceExpectations()
		got, total, totalRecharged, err := service.GetUserBalanceHistory(context.Background(), 685, tc.page, 2, "")
		require.NoError(t, err)
		require.Equal(t, int64(15), total)
		require.Equal(t, 170.0, totalRecharged)
		var codes []string
		for _, item := range got {
			codes = append(codes, item.Code)
		}
		require.Equal(t, tc.want, codes)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBotSalesHistorySumOnlySuccessfulPositiveCredits(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	svc := &adminServiceImpl{entClient: dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))}
	mock.ExpectQuery(`(?s)SUM\(balance_amount\).*FROM bot_sales_fulfillments.*user_id = \$1.*status = 'succeeded'.*balance_amount > 0`).
		WithArgs(int64(685)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(120.0))
	total, err := svc.sumBotSalesBalanceHistory(context.Background(), 685)
	require.NoError(t, err)
	require.Equal(t, 120.0, total)
	require.NoError(t, mock.ExpectationsWereMet())
}
