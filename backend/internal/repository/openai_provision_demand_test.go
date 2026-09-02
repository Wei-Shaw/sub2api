package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGetOpenAIProvisionDemandAggregatesOAuthUsage(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	start := now.Add(-5 * time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("WITH usage_demand AS")).
		WithArgs(service.PlatformOpenAI, service.AccountTypeOAuth, start, now).
		WillReturnRows(sqlmock.NewRows([]string{"active_users", "requests", "tokens"}).AddRow(int64(3), int64(30), int64(900000)))

	repo := newUsageLogRepositoryWithSQL(nil, db)
	demand, err := repo.GetOpenAIProvisionDemand(context.Background(), start, now)

	require.NoError(t, err)
	require.Equal(t, service.OpenAIProvisionDemand{ActiveUsers: 3, Requests: 30, Tokens: 900000}, demand)
	require.NoError(t, mock.ExpectationsWereMet())
}
