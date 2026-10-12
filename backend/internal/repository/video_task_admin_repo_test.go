//go:build unit

package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskAdminRepositoryBoundParametersAndEmptyPage(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := &usageBillingRepository{db: db}
	f := service.VideoTaskAdminFilter{Page: 3, PageSize: 100, Days: 30, Search: `%_' OR TRUE; --\`, Status: "billing_pending"}
	mock.ExpectQuery(regexp.QuoteMeta(videoTaskAdminQuery)).
		WithArgs(30, f.Search, f.Status, 100, int64(200)).
		WillReturnRows(sqlmock.NewRows([]string{"count", "items"}).AddRow(101, `[]`))
	items, total, err := r.ListVideoTasks(context.Background(), f)
	require.NoError(t, err)
	require.EqualValues(t, 101, total, "out-of-range pages must preserve total")
	require.NotNil(t, items)
	require.Empty(t, items)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVideoTaskAdminRepositoryNullableProjection(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta(videoTaskAdminQuery)).
		WithArgs(7, "", "", 20, int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"count", "items"}).AddRow(1,
			`[{"kind":"submission","task_id":"","user_id":null,"api_key_id":42,"model":null,"account_id":null,"expected_cost":null,"charged_cost":null,"billing_status":"review","created_at":"2026-09-15T01:02:03Z","updated_at":"2026-09-15T01:02:03Z"}]`))
	items, total, err := (&usageBillingRepository{db: db}).ListVideoTasks(context.Background(), service.VideoTaskAdminFilter{Page: 1, PageSize: 20, Days: 7})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Nil(t, items[0].UserID)
	require.Nil(t, items[0].Model)
	require.Nil(t, items[0].AccountID)
	require.Nil(t, items[0].ExpectedCost)
	require.Nil(t, items[0].ChargedCost)
	require.Equal(t, "review", items[0].BillingStatus)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVideoTaskAdminRepositoryErrors(t *testing.T) {
	f := service.VideoTaskAdminFilter{Page: 1, PageSize: 20, Days: 7}
	var missing *usageBillingRepository
	_, _, err := missing.ListVideoTasks(context.Background(), f)
	require.ErrorIs(t, err, service.ErrVideoTaskRepositoryUnavailable)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	r := &usageBillingRepository{db: db}
	invalid := f
	invalid.PageSize = 101
	_, _, err = r.ListVideoTasks(context.Background(), invalid)
	require.Error(t, err, "reject before querying")
	for _, tc := range []struct{ name, payload string }{{"database", ""}, {"decode", "{"}} {
		t.Run(tc.name, func(t *testing.T) {
			expect := mock.ExpectQuery(regexp.QuoteMeta(videoTaskAdminQuery))
			if tc.payload == "" {
				expect.WillReturnError(errors.New("database unavailable"))
			} else {
				expect.WillReturnRows(sqlmock.NewRows([]string{"count", "items"}).AddRow(1, tc.payload))
			}
			items, total, err := r.ListVideoTasks(context.Background(), f)
			require.Error(t, err)
			require.Nil(t, items)
			require.Zero(t, total)
		})
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
