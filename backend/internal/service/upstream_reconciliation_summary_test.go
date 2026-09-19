//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestUpstreamReconciliationSummaryRestrictsUserAndOmitsOfficialCosts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &UpstreamReconciliationService{db: db, now: func() time.Time { return time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC) }}
	mock.ExpectQuery(`(?s)SELECT b.currency.*a.user_id=\$2 AND a.method<>'unmatched'`).WithArgs("2026-09", int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"currency", "official", "allocated", "unmatched"}).AddRow("USD", "999", "1.25000000", "998"))
	result, err := s.Summary(context.Background(), "2026-09", 7, false)
	require.NoError(t, err)
	require.Empty(t, result.Bills)
	require.Empty(t, result.Items)
	require.Equal(t, []BillingReportTotal{{Currency: "USD", AllocatedCost: "1.25000000"}}, result.Totals)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpstreamReconciliationSummaryAdminKeepsCurrenciesSeparate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := &UpstreamReconciliationService{db: db, now: time.Now}
	mock.ExpectQuery(`(?s)SELECT currency,SUM\(official\).*UNION ALL`).WithArgs("2025-01").
		WillReturnRows(sqlmock.NewRows([]string{"currency", "official", "allocated", "unmatched"}).
			AddRow("CNY", "9.50000000", "9.00000000", "0.50000000").AddRow("USD", "2.00000000", "1.25000000", "0.75000000"))
	result, err := s.Summary(context.Background(), "2025-01", 7, true)
	require.NoError(t, err)
	require.Len(t, result.Totals, 2)
	require.Equal(t, "9.50000000", result.Totals[0].OfficialCost)
	require.Equal(t, "USD", result.Totals[1].Currency)
	require.NoError(t, mock.ExpectationsWereMet())
}
